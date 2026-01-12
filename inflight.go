// Package inflight provides a duplicate function call suppression mechanism.
//
// It allows multiple concurrent callers for the same key to share the result
// of a single function execution, reducing redundant work and resource usage.
//
// All types in this package are safe for concurrent use by multiple goroutines.
package inflight

import (
	"sync"
	"sync/atomic"

	"github.com/go4org/hashtriemap"
)

// call represents a single in-flight function execution.
// It tracks the number of concurrent callers and ensures the function
// is executed exactly once using [sync.OnceValues].
type call[T any] struct {
	callers  atomic.Int32      // number of callers currently executing [call.do].
	onceFunc func() (T, error) // function wrapped with [sync.OnceValues].
}

// newCall creates a new [call] instance that wraps fn with [sync.OnceValues]
// to ensure it is executed exactly once.
func newCall[T any](fn func() (T, error)) *call[T] {
	return &call[T]{onceFunc: sync.OnceValues(fn)}
}

// do executes the [call.onceFunc] and returns the result along with the number
// of concurrent callers at the time of completion.
// The callers count helps determine if the result is being shared.
func (c *call[T]) do() (T, int32, error) {
	c.callers.Add(1)
	defer c.callers.Add(-1)
	value, err := c.onceFunc()
	return value, c.callers.Load(), err
}

// Group represents a collection of in-flight function calls, keyed by K.
// It deduplicates concurrent calls with the same key, ensuring the function
// is executed only once while sharing the result with all waiters.
//
// Group is safe for concurrent use by multiple goroutines.
// The zero value of Group is ready to use.
type Group[K comparable, V any] struct {
	m hashtriemap.HashTrieMap[K, *call[V]]
}

// Do executes and returns the result of the given function for the specified key,
// ensuring that only one execution is in-flight for concurrent calls with the same key.
// If a duplicate call arrives while the first is still executing, the duplicate caller
// waits for the original to complete and receives the same result.
//
// The returned value is the result of the function execution.
// The returned bool indicates whether the result was shared with other callers (true)
// or if this was the only caller (false). Note that even the first caller may see
// shared=true if other goroutines joined before the function completed.
// The returned error is the error returned by fn, if any.
//
// Do is safe for concurrent use by multiple goroutines.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (V, bool, error) {
	call, loaded := g.m.LoadOrStore(key, newCall(fn))
	if !loaded { // This goroutine stored the [call], it owns the deletion as well.
		defer g.m.CompareAndDelete(key, call)
	}
	value, callers, err := call.do()
	shared := loaded || callers > 1
	return value, shared, err
}

// Forget removes the key from the group's active call registry.
// Future calls to [Group.Do] with this key will execute the function again,
// rather than waiting for or sharing an in-flight call.
//
// If the key has an in-flight call when Forget is called, that call
// continues to execute and serve its existing waiters, but new callers
// will not join it.
//
// Forget is safe for concurrent use by multiple goroutines.
func (g *Group[K, V]) Forget(key K) { g.m.LoadAndDelete(key) }
