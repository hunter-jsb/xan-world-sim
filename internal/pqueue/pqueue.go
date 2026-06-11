// Package pqueue provides a typed min-heap on top of container/heap.
// It exists so the three Dijkstra/priority-flood call sites (road
// building, pit-fill, expedition pathfinding) share one implementation
// instead of each hand-rolling heap.Interface.
//
// It deliberately delegates to container/heap rather than re-deriving
// the sift algorithm: heap layout determines pop order among equal
// keys, and world generation's snapshot determinism depends on that
// order staying exactly as it was.
package pqueue

import "container/heap"

// Heap is a min-heap ordered by the less function given to New.
type Heap[T any] struct {
	inner inner[T]
}

func New[T any](less func(a, b T) bool) *Heap[T] {
	return &Heap[T]{inner: inner[T]{less: less}}
}

func (h *Heap[T]) Len() int { return len(h.inner.items) }
func (h *Heap[T]) Push(v T) { heap.Push(&h.inner, v) }
func (h *Heap[T]) Pop() T   { return heap.Pop(&h.inner).(T) }

// inner implements heap.Interface.
type inner[T any] struct {
	items []T
	less  func(a, b T) bool
}

func (in inner[T]) Len() int           { return len(in.items) }
func (in inner[T]) Less(i, j int) bool { return in.less(in.items[i], in.items[j]) }
func (in inner[T]) Swap(i, j int)      { in.items[i], in.items[j] = in.items[j], in.items[i] }

func (in *inner[T]) Push(x any) { in.items = append(in.items, x.(T)) }

func (in *inner[T]) Pop() any {
	old := in.items
	n := len(old)
	x := old[n-1]
	in.items = old[:n-1]
	return x
}
