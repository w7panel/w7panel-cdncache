package logic

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	cacheExpirationDBName         = "expiration.db"
	cacheExpirationBucketName     = "tasks"
	cacheExpirationWriteBatchSize = 1_000
	cacheExpirationWriteQueueSize = 256
	cacheExpirationSyncInterval   = 5 * time.Second
)

var errCacheExpirationStoreClosed = errors.New("cache expiration store is closed")

type cacheExpirationEvent struct {
	operation string
	task      CacheExpirationTask
}

// cacheExpirationPersistence is the narrow persistence contract used by the
// scheduler. bbolt transactions, batching, and syncing stay inside
// cacheExpirationStore.
type CacheExpirationPersistence interface {
	SaveTasks([]CacheExpirationTask) error
	DeleteTasks([]CacheExpirationTask) error
	TasksSnapshot() ([]CacheExpirationTask, error)
	Close() error
}

// cacheExpirationStore owns the bbolt database and the asynchronous writer.
// The queue only sees save, delete, snapshot, and close operations; it does
// not need to know anything about bbolt transactions or files.
type CacheExpirationStore struct {
	submitMu  sync.Mutex // serializes event order and Close with submitters
	closeOnce sync.Once
	closed    bool
	closeErr  error

	db *bolt.DB

	writeCh   chan []cacheExpirationEvent
	writeDone chan struct{}
	writerErr error
	dirty     bool
}

func NewCacheExpirationStore(storeDir string) (*CacheExpirationStore, error) {
	if storeDir == "" {
		return nil, errors.New("cache expiration store directory is empty")
	}
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache expiration store directory: %w", err)
	}

	dbPath := filepath.Join(storeDir, cacheExpirationDBName)
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: time.Second,
		NoSync:  true, // writer syncs periodically instead of on every batch
	})
	if err != nil {
		return nil, fmt.Errorf("open cache expiration bbolt database: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(cacheExpirationBucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create cache expiration bbolt bucket: %w", err)
	}

	store := &CacheExpirationStore{
		db:        db,
		writeCh:   make(chan []cacheExpirationEvent, cacheExpirationWriteQueueSize),
		writeDone: make(chan struct{}),
	}
	go store.writeLoop()
	return store, nil
}

// writeLoop collects mutations and commits them in one bbolt transaction.
// Batches flush immediately at the size limit or on the sync ticker. Sync is
// intentionally less frequent because the task store is a cache cleanup aid,
// not request data.
func (s *CacheExpirationStore) writeLoop() {
	syncTicker := time.NewTicker(cacheExpirationSyncInterval)
	defer syncTicker.Stop()
	defer close(s.writeDone)

	var pending []cacheExpirationEvent
	for {
		select {
		case events, ok := <-s.writeCh:
			if !ok {
				for events := range s.writeCh {
					pending = append(pending, events...)
				}
				for len(pending) > 0 {
					before := len(pending)
					pending = s.flushPending(pending)
					if len(pending) == before {
						break
					}
				}
				if err := s.syncDB(); err != nil {
					s.writerErr = err
				}
				return
			}
			pending = append(pending, events...)
			if len(pending) >= cacheExpirationWriteBatchSize {
				pending = s.flushPending(pending)
			}
		case <-syncTicker.C:
			pending = s.flushPending(pending)
			if len(pending) == 0 {
				if err := s.syncDB(); err != nil {
					s.writerErr = err
					slog.Error("sync cache expiration bbolt database", "err", err)
				}
			}
		}
	}
}

func (s *CacheExpirationStore) flushPending(pending []cacheExpirationEvent) []cacheExpirationEvent {
	if len(pending) == 0 {
		return nil
	}
	putCount, deleteCount := cacheExpirationEventCounts(pending)
	if err := s.writeEvents(pending); err != nil {
		s.writerErr = err
		slog.Error("write cache expiration bbolt batch",
			"count", len(pending),
			"put", putCount,
			"delete", deleteCount,
			"err", err,
		)
		return pending
	}
	s.dirty = true
	s.writerErr = nil
	slog.Info("write cache expiration bbolt batch",
		"count", len(pending),
		"put", putCount,
		"delete", deleteCount,
	)
	return nil
}

func (s *CacheExpirationStore) writeEvents(events []cacheExpirationEvent) error {
	if s.db == nil {
		return errors.New("cache expiration bbolt database is not open")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(cacheExpirationBucketName))
		if bucket == nil {
			return errors.New("cache expiration task bucket is missing")
		}
		for _, event := range events {
			key := []byte(expirationTaskKey(event.task))
			switch event.operation {
			case "put":
				value, err := json.Marshal(event.task)
				if err != nil {
					return fmt.Errorf("encode cache expiration task: %w", err)
				}
				if err := bucket.Put(key, value); err != nil {
					return fmt.Errorf("put cache expiration task: %w", err)
				}
			case "delete":
				if err := bucket.Delete(key); err != nil {
					return fmt.Errorf("delete cache expiration task: %w", err)
				}
			default:
				return fmt.Errorf("unknown cache expiration operation %q", event.operation)
			}
		}
		return nil
	})
}

func (s *CacheExpirationStore) syncDB() error {
	if !s.dirty {
		return nil
	}
	if s.db == nil {
		return errors.New("cache expiration bbolt database is not open")
	}
	if err := s.db.Sync(); err != nil {
		return fmt.Errorf("sync cache expiration bbolt database: %w", err)
	}
	s.dirty = false
	s.writerErr = nil
	slog.Info("sync cache expiration bbolt database")
	return nil
}

func (s *CacheExpirationStore) SaveTasks(tasks []CacheExpirationTask) error {
	if len(tasks) == 0 {
		return nil
	}
	events := make([]cacheExpirationEvent, 0, len(tasks))
	for i := range tasks {
		if err := validateExpirationTask(tasks[i]); err != nil {
			return err
		}
		tasks[i].ExpireAt = tasks[i].ExpireAt.UTC()
		events = append(events, cacheExpirationEvent{operation: "put", task: tasks[i]})
	}
	return s.enqueue(events)
}

func (s *CacheExpirationStore) DeleteTasks(tasks []CacheExpirationTask) error {
	if len(tasks) == 0 {
		return nil
	}
	events := make([]cacheExpirationEvent, 0, len(tasks))
	for i := range tasks {
		if err := validateExpirationTask(tasks[i]); err != nil {
			return err
		}
		events = append(events, cacheExpirationEvent{operation: "delete", task: tasks[i]})
	}
	return s.enqueue(events)
}

func (s *CacheExpirationStore) enqueue(events []cacheExpirationEvent) error {
	s.submitMu.Lock()
	defer s.submitMu.Unlock()
	if s.closed {
		return errCacheExpirationStoreClosed
	}
	s.writeCh <- events
	return nil
}

// tasksSnapshot reads all task values once when the queue starts and uses them
// to rebuild its in-memory expiration heap.
func (s *CacheExpirationStore) TasksSnapshot() ([]CacheExpirationTask, error) {
	if s.db == nil {
		return nil, errors.New("cache expiration bbolt database is not open")
	}
	tasks := make([]CacheExpirationTask, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(cacheExpirationBucketName))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, value []byte) error {
			if value == nil {
				return nil
			}
			var task CacheExpirationTask
			if err := json.Unmarshal(value, &task); err != nil {
				return fmt.Errorf("decode cache expiration task: %w", err)
			}
			if err := validateExpirationTask(task); err != nil {
				return fmt.Errorf("invalid cache expiration task: %w", err)
			}
			tasks = append(tasks, task)
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("load cache expiration tasks: %w", err)
	}
	slog.Info("load cache expiration tasks from bbolt", "count", len(tasks))
	return tasks, nil
}

// validateExpirationTask validates fields required by the persisted task.
func validateExpirationTask(task CacheExpirationTask) error {
	if task.Host == "" || task.Bucket == "" || task.Key == "" {
		return errors.New("host, bucket and key are required")
	}
	if task.ExpireAt.IsZero() {
		return errors.New("expire_at is required")
	}
	return nil
}

// expirationTaskKey identifies one host, bucket, and object key.
func expirationTaskKey(task CacheExpirationTask) string {
	return task.Host + "\x00" + task.Bucket + "\x00" + task.Key
}

func (s *CacheExpirationStore) Close() error {
	s.closeOnce.Do(func() {
		s.submitMu.Lock()
		s.closed = true
		close(s.writeCh)
		s.submitMu.Unlock()
		<-s.writeDone
		if s.db != nil {
			s.closeErr = errors.Join(s.writerErr, s.db.Close())
		} else {
			s.closeErr = s.writerErr
		}
	})
	if s.closeErr != nil {
		slog.Error("close cache expiration bbolt store", "err", s.closeErr)
	} else {
		slog.Info("close cache expiration bbolt store")
	}
	return s.closeErr
}

func cacheExpirationEventCounts(events []cacheExpirationEvent) (putCount, deleteCount int) {
	for _, event := range events {
		switch event.operation {
		case "put":
			putCount++
		case "delete":
			deleteCount++
		}
	}
	return putCount, deleteCount
}
