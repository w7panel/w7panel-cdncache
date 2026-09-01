package helper

import stdheap "container/heap"

// MinHeap is a binary min-heap. The caller supplies the ordering function;
// less(a, b) must return true when a has a smaller priority than b.
//
// MinHeap is not safe for concurrent use. Callers that share a heap between
// goroutines must provide their own synchronization.
type MinHeap[T any] struct {
	items minHeapItems[T]
}

// NewMinHeap creates an empty min-heap ordered by less.
func NewMinHeap[T any](less func(a, b T) bool) *MinHeap[T] {
	if less == nil {
		panic("helper: min heap comparator must not be nil")
	}
	return &MinHeap[T]{items: minHeapItems[T]{less: less}}
}

// Len returns the number of values in the heap.
func (h *MinHeap[T]) Len() int {
	if h == nil {
		return 0
	}
	return h.items.Len()
}

// Peek returns the value with the smallest priority without removing it.
func (h *MinHeap[T]) Peek() (T, bool) {
	if h == nil || h.Len() == 0 {
		var zero T
		return zero, false
	}
	return h.items.items[0], true
}

// Push adds value to the heap.
func (h *MinHeap[T]) Push(value T) {
	stdheap.Push(&h.items, value)
}

// Pop removes and returns the value with the smallest priority.
func (h *MinHeap[T]) Pop() (T, bool) {
	if h == nil || h.Len() == 0 {
		var zero T
		return zero, false
	}
	return stdheap.Pop(&h.items).(T), true
}

// minHeapItems adapts the generic storage to container/heap while keeping the
// interface{} implementation detail private to this package.
type minHeapItems[T any] struct {
	items []T
	less  func(a, b T) bool
}

func (h minHeapItems[T]) Len() int { return len(h.items) }

func (h minHeapItems[T]) Less(i, j int) bool {
	return h.less(h.items[i], h.items[j])
}

func (h minHeapItems[T]) Swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *minHeapItems[T]) Push(value interface{}) {
	h.items = append(h.items, value.(T))
}

func (h *minHeapItems[T]) Pop() interface{} {
	last := len(h.items) - 1
	item := h.items[last]
	var zero T
	h.items[last] = zero
	h.items = h.items[:last]
	return item
}
