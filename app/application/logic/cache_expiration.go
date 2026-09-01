package logic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/we7coreteam/w7-cdn-cache/common/helper"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/panjf2000/ants/v2"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"go.uber.org/zap"
)

const (
	cacheExpirationDirName        = "cache-expiration"
	cacheExpirationBatchSize      = 1_000
	cacheExpirationWorkerNum      = 4
	cacheExpirationRequestTimeout = 30 * time.Second
)

// CacheExpirationTask identifies one cached object and its expiration time.
// VersionID, when available, targets the exact uploaded object version.
type CacheExpirationTask struct {
	Host      string `json:"host"`
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	VersionID string `json:"version_id,omitempty"`
	// StorageFingerprint identifies the S3 target used when the object was uploaded.
	StorageFingerprint string    `json:"storage_fingerprint,omitempty"`
	ExpireAt           time.Time `json:"expire_at"`
}

type expirationHeapItem struct {
	Task CacheExpirationTask
}

// CacheExpirationQueue is a durable, single-process delayed deletion queue.
// It only processes tasks persisted by the application and never scans S3 to
// discover objects that were not registered.
type CacheExpirationQueue struct {
	// mu protects the scheduler heap. Durable task state is owned and locked by
	// store, so queue scheduling never reaches into the store's internals.
	mu    sync.Mutex
	store CacheExpirationPersistence
	heap  *helper.MinHeap[*expirationHeapItem]
	pool  *ants.PoolWithFunc
	wake  chan struct{}
}

type cacheExpirationWork struct {
	ctx   context.Context
	tasks []CacheExpirationTask
}

var defaultQueue atomic.Pointer[CacheExpirationQueue]

func StartCacheExpirationClearLoop() {
	go func() {
		err := StartCacheExpirationWorker()
		if err != nil {
			slog.Error("start cache expiration worker failed", zap.Error(err))
			return
		}
	}()
}

// StartCacheExpirationWorker opens the persisted queue and starts its worker.
func StartCacheExpirationWorker() error {
	if !cacheExpirationEnabled() {
		return nil
	}

	store, err := NewCacheExpirationStore(facade.GetConfig().GetString("cache.cleanup.queue_dir"))
	if err != nil {
		return err
	}
	queue, err := NewCacheExpirationQueue(store)
	if err != nil {
		return err
	}
	if !defaultQueue.CompareAndSwap(nil, queue) {
		_ = queue.Close()
		return nil
	}

	queue.Run(context.Background())
	return nil
}

func cacheExpirationEnabled() bool {
	config := facade.GetConfig()
	return config == nil || !config.IsSet("cache.cleanup.enabled") || config.GetBool("cache.cleanup.enabled")
}

// Queue API.
// Queue construction and process-level lifecycle.

// NewCacheExpirationQueue opens the durable store and initializes the
// in-memory scheduler and cleanup workers. Persistence remains independent of
// the queue so the store can be tested and evolved without scheduler state.
func NewCacheExpirationQueue(store CacheExpirationPersistence) (*CacheExpirationQueue, error) {
	queue := &CacheExpirationQueue{
		store: store,
		heap: helper.NewMinHeap(func(a, b *expirationHeapItem) bool {
			return a.Task.ExpireAt.Before(b.Task.ExpireAt)
		}),
		wake: make(chan struct{}, 1),
	}
	tasks, err := store.TasksSnapshot()
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("load cache expiration tasks: %w", err)
	}
	for _, task := range tasks {
		queue.pushHeapItemLocked(task)
	}

	queue.pool, err = ants.NewPoolWithFunc(cacheExpirationWorkerNum, func(value interface{}) {
		work := value.(cacheExpirationWork)
		queue.processTaskGroup(work.ctx, work.tasks)
	})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create cache expiration worker pool: %w", err)
	}
	return queue, nil
}

// EnqueueCacheExpiration persists a task and wakes the worker to recalculate
// the next deadline.
func EnqueueCacheExpiration(task CacheExpirationTask) error {
	if !cacheExpirationEnabled() {
		return nil
	}
	queue := defaultQueue.Load()
	if queue == nil {
		return errors.New("cache expiration worker is not started")
	}
	return queue.Enqueue(task)
}

func (q *CacheExpirationQueue) Enqueue(task CacheExpirationTask) error {
	return q.EnqueueMany([]CacheExpirationTask{task})
}

func (q *CacheExpirationQueue) EnqueueMany(tasks []CacheExpirationTask) error {
	if len(tasks) == 0 {
		return nil
	}
	for i := range tasks {
		tasks[i].ExpireAt = tasks[i].ExpireAt.UTC()
	}

	if err := q.store.SaveTasks(tasks); err != nil {
		return err
	}

	q.mu.Lock()
	for _, task := range tasks {
		q.pushHeapItemLocked(task)
	}
	q.mu.Unlock()

	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

// Queue scheduler.

// Run blocks until ctx is canceled and processes due tasks in batches.
func (q *CacheExpirationQueue) Run(ctx context.Context) {
	for {
		tasks, wait, hasNext := q.takeDue(time.Now(), cacheExpirationBatchSize)
		if len(tasks) > 0 {
			q.dispatchTasks(ctx, tasks)
			continue
		}

		if !hasNext {
			// There is no pending deadline. Sleep until a new task wakes the
			// scheduler, or until the worker context is canceled.
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			}
			continue
		}

		// A future task remains in the heap. Wait exactly until its deadline;
		// q.wake interrupts this timer when an earlier task is enqueued.
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-q.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

// dispatchTasks submits one independent host/bucket group to the cleanup
// pool. Invoke applies backpressure when all cleanup workers are busy, keeping
// the in-memory work queue bounded.
func (q *CacheExpirationQueue) dispatchTasks(ctx context.Context, tasks []CacheExpirationTask) {
	// DeleteObjects accepts only one bucket per request. Grouping by host and
	// bucket keeps each batch on the correct S3 client and allows independent
	// storage targets to be processed concurrently by the cleanup pool.
	groups := make(map[string][]CacheExpirationTask)
	for _, task := range tasks {
		groupKey := task.Host + "\x00" + task.Bucket
		groups[groupKey] = append(groups[groupKey], task)
	}
	for _, group := range groups {
		if err := q.pool.Invoke(cacheExpirationWork{ctx: ctx, tasks: group}); err != nil {
			slog.Error("dispatch cache expiration tasks", "err", err)
		}
	}
}

// pushHeapItemLocked adds a scheduling entry. Entries are intentionally
// append-only because duplicate cache deletions are harmless.
// The caller must hold q.mu.
func (q *CacheExpirationQueue) pushHeapItemLocked(task CacheExpirationTask) {
	q.heap.Push(&expirationHeapItem{
		Task: task,
	})
}

// takeDue returns due tasks, the delay until the next task, and whether a
// future task remains in the heap. The boolean replaces a negative-duration
// sentinel for an empty heap.
func (q *CacheExpirationQueue) takeDue(now time.Time, limit int) ([]CacheExpirationTask, time.Duration, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	tasks := make([]CacheExpirationTask, 0, limit)
	for q.heap.Len() > 0 && len(tasks) < limit {
		// Pop always returns the earliest deadline in the min-heap. The Pop
		// result only tells us that an item existed; expiration is determined by
		// comparing its deadline with the single timestamp captured by the caller.
		item, ok := q.heap.Pop()
		if !ok {
			break
		}
		if item.Task.ExpireAt.After(now) {
			// The earliest task is still in the future. Put it back because Pop
			// removed it, then stop: every later heap item expires no earlier.
			q.heap.Push(item)
			break
		}
		// ExpireAt <= now means this task is due and can be dispatched.
		tasks = append(tasks, item.Task)
	}
	if len(tasks) > 0 {
		return tasks, 0, q.heap.Len() > 0
	}
	if q.heap.Len() == 0 {
		return nil, 0, false
	}
	// Peek does not remove the task. It only reads the next earliest deadline
	// so Run can sleep until that deadline instead of repeatedly polling.
	item, ok := q.heap.Peek()
	if !ok {
		return nil, 0, false
	}
	wait := time.Until(item.Task.ExpireAt)
	if wait < 0 {
		wait = 0
	}
	return nil, wait, true
}

// Cleanup execution.

func (q *CacheExpirationQueue) processTaskGroup(ctx context.Context, tasks []CacheExpirationTask) {
	if len(tasks) == 0 {
		return
	}

	setting := (Setting{}).GetStorageCacheSettingByHost(tasks[0].Host)
	if setting.StorageCacheS3 == nil || setting.StorageCacheS3.Endpoint == "" {
		slog.Error("cache expiration storage configuration is not available", "host", tasks[0].Host)
		return
	}

	client, clientFingerprint := (S3Client{}).GetS3ClientWithFingerprint(tasks[0].Host)
	if client == nil {
		slog.Error("cache expiration s3 client is not configured", "host", tasks[0].Host)
		return
	}

	currentFingerprint := storageConfigFingerprint(*setting.StorageCacheS3)
	deletable := make([]CacheExpirationTask, 0, len(tasks))
	completed := make([]CacheExpirationTask, 0, len(tasks))
	for _, task := range tasks {
		// Legacy tasks have no storage identity, so they must not be deleted
		// against a potentially different backend after a configuration change.
		if task.StorageFingerprint == "" || task.StorageFingerprint != currentFingerprint || task.StorageFingerprint != clientFingerprint || task.Bucket != setting.StorageCacheS3.Bucket {
			slog.Warn("skip stale cache expiration task", "host", task.Host, "bucket", task.Bucket, "key", task.Key)
			completed = append(completed, task)
			continue
		}
		deletable = append(deletable, task)
	}
	if len(deletable) == 0 {
		q.deleteTasks(completed)
		return
	}

	objects := make([]types.ObjectIdentifier, 0, len(deletable))
	for _, task := range deletable {
		object := types.ObjectIdentifier{Key: aws.String(task.Key)}
		if task.VersionID != "" {
			object.VersionId = aws.String(task.VersionID)
		}
		objects = append(objects, object)
	}
	// Bound the complete cleanup operation, including any retries performed by
	// the S3 SDK. The cleanup worker uses a background context at startup, so
	// without this deadline a stalled endpoint could occupy a worker forever.
	requestCtx, cancel := context.WithTimeout(ctx, cacheExpirationRequestTimeout)
	defer cancel()
	removeOutput, err := client.DeleteObjects(requestCtx, &s3.DeleteObjectsInput{
		Bucket: aws.String(deletable[0].Bucket),
		Delete: &types.Delete{Objects: objects},
	})
	if err == nil && removeOutput == nil {
		err = errors.New("s3 delete returned an empty response")
	}
	removeErrors := make(map[string]error)
	if err != nil {
		for _, task := range deletable {
			removeErrors[task.Key] = err
		}
	} else {
		for _, removeErr := range removeOutput.Errors {
			key := aws.ToString(removeErr.Key)
			removeErrors[key] = s3DeleteError{
				code:    aws.ToString(removeErr.Code),
				message: aws.ToString(removeErr.Message),
			}
		}
	}
	for _, task := range deletable {
		if err, ok := removeErrors[task.Key]; ok {
			// Keep every failed item in the store. We intentionally do not classify
			// S3 errors here; if retries are needed later, they belong around the
			// DeleteObjects call rather than in the scheduling queue.
			slog.Error("delete cache expiration object", "host", task.Host, "bucket", task.Bucket, "key", task.Key, "err", err)
			continue
		}
		completed = append(completed, task)
	}
	q.deleteTasks(completed)
}

func (q *CacheExpirationQueue) deleteTasks(tasks []CacheExpirationTask) {
	if len(tasks) == 0 {
		return
	}
	if err := q.store.DeleteTasks(tasks); err != nil {
		slog.Error("persist cache expiration completion", "tasks", len(tasks), "err", err)
	}
}

// Close stops accepting cleanup work and closes the durable store. The caller
// should cancel the context passed to Run before closing the queue.
func (q *CacheExpirationQueue) Close() error {
	if q.pool != nil {
		q.pool.Release()
	}
	if q.store != nil {
		return q.store.Close()
	}
	return nil
}

// Shared cache TTL and S3 cleanup helpers.

func cacheTTLDuration(ttl int64) (time.Duration, bool) {
	if ttl <= 0 || ttl > math.MaxInt64/int64(time.Minute) {
		return 0, false
	}
	return time.Duration(ttl) * time.Minute, true
}

// s3DeleteError represents an item-level error returned by DeleteObjects.
// It is intentionally kept local to the expiration queue so callers only see
// ordinary errors and do not depend on generated SDK response types.
type s3DeleteError struct {
	code    string
	message string
}

func (e s3DeleteError) Error() string {
	if e.message == "" {
		return e.code
	}
	return e.code + ": " + e.message
}
