package pqueue

import (
	"math/rand"
	"sort"
	"testing"
)

func TestHeap_PopsInOrder(t *testing.T) {
	h := New(func(a, b int) bool { return a < b })
	in := []int{5, 3, 8, 1, 9, 2, 7, 2, 5}
	for _, v := range in {
		h.Push(v)
	}
	if h.Len() != len(in) {
		t.Fatalf("Len = %d, want %d", h.Len(), len(in))
	}
	want := append([]int(nil), in...)
	sort.Ints(want)
	for i, w := range want {
		if got := h.Pop(); got != w {
			t.Fatalf("pop %d = %d, want %d", i, got, w)
		}
	}
	if h.Len() != 0 {
		t.Fatalf("Len after draining = %d, want 0", h.Len())
	}
}

func TestHeap_InterleavedPushPop(t *testing.T) {
	h := New(func(a, b int) bool { return a < b })
	h.Push(4)
	h.Push(1)
	if got := h.Pop(); got != 1 {
		t.Fatalf("pop = %d, want 1", got)
	}
	h.Push(0)
	h.Push(9)
	if got := h.Pop(); got != 0 {
		t.Fatalf("pop = %d, want 0", got)
	}
	if got := h.Pop(); got != 4 {
		t.Fatalf("pop = %d, want 4", got)
	}
	if got := h.Pop(); got != 9 {
		t.Fatalf("pop = %d, want 9", got)
	}
}

func TestHeap_StructWithTiebreak(t *testing.T) {
	type item struct{ dist, y, x int }
	less := func(a, b item) bool {
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		if a.y != b.y {
			return a.y < b.y
		}
		return a.x < b.x
	}
	h := New(less)
	rng := rand.New(rand.NewSource(1))
	var in []item
	for i := 0; i < 200; i++ {
		it := item{rng.Intn(10), rng.Intn(10), rng.Intn(10)}
		in = append(in, it)
		h.Push(it)
	}
	sort.Slice(in, func(i, j int) bool { return less(in[i], in[j]) })
	for i, w := range in {
		if got := h.Pop(); got != w {
			t.Fatalf("pop %d = %+v, want %+v", i, got, w)
		}
	}
}
