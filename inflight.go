package inflight

import (
	"sync"
	"sync/atomic"

	"github.com/go4org/hashtriemap"
)

// call
type call[T any] struct {
	callers atomic.Int32
	fn      func() (T, error)
}

// newCall
func newCall[T any](fn func() (T, error)) *call[T] {
	return &call[T]{fn: sync.OnceValues(fn)}
}

func (c *call[T]) do() (T, int32, error) {
	c.callers.Add(1)
	defer c.callers.Add(-1)
	value, err := c.fn()
	return value, c.callers.Load(), err
}

// Group
type Group[K comparable, V any] struct {
	m hashtriemap.HashTrieMap[K, *call[V]]
}

// Do
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (V, bool, error) {
	call, loaded := g.m.LoadOrStore(key, newCall(fn))
	value, callers, err := call.do()
	if !loaded {
		g.m.CompareAndDelete(key, call)
	}
	shared := loaded || callers > 1
	return value, shared, err
}

// Forget
func (g *Group[K, V]) Forget(key K) { g.m.LoadAndDelete(key) }
