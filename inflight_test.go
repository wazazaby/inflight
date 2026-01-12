package inflight

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDo(t *testing.T) {
	var g Group[string, string]
	v, shared, err := g.Do("key", func() (string, error) {
		return "bar", nil
	})
	require.Equal(t, "bar (string)", fmt.Sprintf("%v (%T)", v, v))
	require.False(t, shared)
	require.NoError(t, err)
}

func TestDoErr(t *testing.T) {
	var g Group[string, string]
	someErr := errors.New("some error")
	v, shared, err := g.Do("key", func() (string, error) {
		return "", someErr
	})
	require.ErrorIs(t, err, someErr)
	require.False(t, shared)
	require.Empty(t, v)
}

func TestDoDupSuppress(t *testing.T) {
	var g Group[string, string]

	const n = 16

	var nbCalls atomic.Int32
	var nbShared atomic.Int32

	c := make(chan string, n)

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			v, shared, err := g.Do("key", func() (string, error) {
				nbCalls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "bar", nil
			})
			require.NoError(t, err)
			if shared {
				nbShared.Add(1)
			}
			c <- v
		})
	}

	wg.Wait()
	close(c)

	require.Equal(t, int32(1), nbCalls.Load())

	// At least n-1 calls should be marked as shared.
	require.GreaterOrEqual(t, nbShared.Load(), int32(n-1))

	t.Logf("%d calls were shared", nbShared.Load())

	for v := range c {
		require.Equal(t, "bar", v)
	}
}

func TestForgetDuringCall(t *testing.T) {
	var g Group[string, string]

	var firstStarted sync.WaitGroup
	firstStarted.Add(1)

	c1 := make(chan struct{})
	go func() {
		g.Do("key", func() (string, error) {
			firstStarted.Done()
			<-c1
			return "first", nil
		})
	}()

	firstStarted.Wait()

	// Forget while the first call is still running (blocked on reading c1).
	g.Forget("key")

	// A second call should not share the first call after Forget.
	var calls atomic.Int32
	v, shared, err := g.Do("key", func() (string, error) {
		calls.Add(1)
		return "second", nil
	})

	close(c1)

	require.NoError(t, err)
	require.Equal(t, "second", v)
	require.False(t, shared)
	require.Equal(t, int32(1), calls.Load())
}

func TestConcurrentDifferentKeys(t *testing.T) {
	var g Group[string, string]
	const n = 32

	var nbExecs atomic.Uint32

	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			k := fmt.Sprintf("key%d", i)
			g.Do(k, func() (string, error) {
				nbExecs.Add(1)
				time.Sleep(10 * time.Millisecond)
				return k, nil
			})
		})
	}

	wg.Wait()

	require.Equal(t, uint32(n), nbExecs.Load())
}

func TestSharedFlag(t *testing.T) {
	var g Group[string, int]

	// Test 1: Concurrent calls should all report shared=true.
	t.Run("concurrent_calls", func(t *testing.T) {
		block := make(chan struct{})

		var wg sync.WaitGroup
		const n = 16

		for range n {
			wg.Go(func() {
				_, shared, err := g.Do("key1", func() (int, error) {
					<-block
					return 42, nil
				})
				require.NoError(t, err)
				require.True(t, shared)
			})
		}

		time.Sleep(50 * time.Millisecond) // Ensure all goroutines are waiting.
		close(block)
		wg.Wait()
	})

	// Test 2: Sequential calls should report shared=false.
	t.Run("sequential_calls", func(t *testing.T) {
		_, shared1, err := g.Do("key2", func() (int, error) {
			return 1234, nil
		})
		require.False(t, shared1)
		require.NoError(t, err)
		t.Logf("first call shared=%v", shared1)

		_, shared2, err := g.Do("key2", func() (int, error) {
			return 5678, nil
		})
		require.False(t, shared2)
		require.NoError(t, err)
		t.Logf("second call shared=%v", shared2)
	})
}
