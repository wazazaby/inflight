package inflight

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	var g Group[string, string]
	v, shared, err := g.Do("key", func() (string, error) {
		return "bar", nil
	})
	if got, want := fmt.Sprintf("%v (%T)", v, v), "bar (string)"; got != want {
		t.Errorf("Do = %v; want %v", got, want)
	}
	if shared {
		t.Errorf("shared = %t; want false", shared)
	}
	if err != nil {
		t.Errorf("Do error = %v", err)
	}
}

func TestDoErr(t *testing.T) {
	var g Group[string, string]
	someErr := errors.New("some error")
	v, shared, err := g.Do("key", func() (string, error) {
		return "", someErr
	})
	if err != someErr {
		t.Errorf("Do error = %v; want someErr %v", err, someErr)
	}
	if shared {
		t.Errorf("shared = %t; want false", shared)
	}
	if v != "" {
		t.Errorf("unexpected non-zero value %#v", v)
	}
}

func TestDoDupSuppress(t *testing.T) {
	var g Group[string, string]
	var calls atomic.Int32
	const n = 10
	var wg sync.WaitGroup
	c := make(chan string, n)
	var sharedCount atomic.Int32

	for range n {
		wg.Go(func() {
			v, shared, err := g.Do("key", func() (string, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "bar", nil
			})
			if err != nil {
				t.Errorf("Do error: %v", err)
				return
			}
			if shared {
				sharedCount.Add(1)
			}
			c <- v
		})
	}

	wg.Wait()
	close(c)

	if got := calls.Load(); got != 1 {
		t.Errorf("number of calls = %d; want 1", got)
	}

	// At least n-1 calls should be marked as shared
	if got := sharedCount.Load(); got < int32(n-1) {
		t.Errorf("number of shared calls = %d; want at least %d", got, n-1)
	}

	for v := range c {
		if v != "bar" {
			t.Errorf("got %q; want %q", v, "bar")
		}
	}
}

func TestForget(t *testing.T) {
	var g Group[string, int]
	var calls atomic.Int32

	key := "key"
	fn := func() (int, error) {
		return int(calls.Add(1)), nil
	}

	v1, shared, err := g.Do(key, fn)
	if err != nil {
		t.Errorf("Do error: %v", err)
	}
	if shared {
		t.Errorf("first call should not be shared")
	}
	if v1 != 1 {
		t.Errorf("got %d; want 1", v1)
	}

	// Without Forget, concurrent call during execution might share
	// But after completion, new calls would not share
	// To test Forget properly, we need to ensure the key is cleared

	// Wait a bit to ensure the first call completes
	time.Sleep(10 * time.Millisecond)

	// Forget the key
	g.Forget(key)

	// Now a new call should execute the function again
	v2, shared, err := g.Do(key, fn)
	if err != nil {
		t.Errorf("Do error: %v", err)
	}
	if shared {
		t.Errorf("after Forget, call should not be shared")
	}
	if v2 != 2 {
		t.Errorf("got %d; want 2 (function should be called again)", v2)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("number of calls = %d; want 2", got)
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

	// Forget while the first call is still running
	g.Forget("key")

	// A second call should not share the first call after Forget
	var calls atomic.Int32
	v, _, err := g.Do("key", func() (string, error) {
		calls.Add(1)
		return "second", nil
	})

	close(c1)

	if err != nil {
		t.Errorf("Do error: %v", err)
	}
	if v != "second" {
		t.Errorf("got %q; want %q", v, "second")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("second function was called %d times; want 1", got)
	}
}

// TestDoGenericTypes tests Do with various generic types
func TestDoGenericTypes(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		var g Group[string, int]
		v, _, err := g.Do("key", func() (int, error) {
			return 42, nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		if v != 42 {
			t.Errorf("got %d; want 42", v)
		}
	})

	t.Run("struct", func(t *testing.T) {
		type result struct {
			Name  string
			Value int
		}
		var g Group[string, result]
		want := result{Name: "test", Value: 123}
		v, _, err := g.Do("key", func() (result, error) {
			return want, nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		if v != want {
			t.Errorf("got %+v; want %+v", v, want)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		type data struct {
			Value int
		}
		var g Group[string, *data]
		want := &data{Value: 99}
		v, _, err := g.Do("key", func() (*data, error) {
			return want, nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		if v != want {
			t.Errorf("got %p; want %p", v, want)
		}
		if v.Value != 99 {
			t.Errorf("got value %d; want 99", v.Value)
		}
	})

	t.Run("slice", func(t *testing.T) {
		var g Group[int, []string]
		want := []string{"a", "b", "c"}
		v, _, err := g.Do(1, func() ([]string, error) {
			return want, nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		if len(v) != len(want) {
			t.Errorf("got length %d; want %d", len(v), len(want))
		}
		for i := range v {
			if v[i] != want[i] {
				t.Errorf("got v[%d] = %q; want %q", i, v[i], want[i])
			}
		}
	})
}

// TestConcurrentDifferentKeys tests that different keys can execute concurrently
func TestConcurrentDifferentKeys(t *testing.T) {
	var g Group[string, string]
	const n = 10

	var wg sync.WaitGroup
	var mu sync.Mutex
	execTimes := make(map[string]time.Time)

	for i := 0; i < n; i++ {
		wg.Add(1)
		key := fmt.Sprintf("key%d", i)
		go func(k string) {
			defer wg.Done()
			g.Do(k, func() (string, error) {
				mu.Lock()
				execTimes[k] = time.Now()
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				return k, nil
			})
		}(key)
	}

	wg.Wait()

	if len(execTimes) != n {
		t.Errorf("expected %d different keys to execute, got %d", n, len(execTimes))
	}
}

// TestSharedFlag tests the shared return value
func TestSharedFlag(t *testing.T) {
	var g Group[string, int]

	// Test 1: Concurrent calls should all report shared=true
	t.Run("concurrent_calls", func(t *testing.T) {
		started := make(chan struct{})
		block := make(chan struct{})

		var wg sync.WaitGroup
		const n = 5
		results := make([]bool, n)

		for i := range n {
			wg.Add(1)
			idx := i
			go func() {
				defer wg.Done()
				_, shared, err := g.Do("key1", func() (int, error) {
					if idx == 0 {
						close(started)
					}
					<-block
					return 42, nil
				})
				if err != nil {
					t.Errorf("Do error: %v", err)
				}
				results[idx] = shared
			}()
			if i == 0 {
				<-started
				time.Sleep(10 * time.Millisecond) // Give first goroutine a head start
			}
		}

		time.Sleep(50 * time.Millisecond) // Ensure all goroutines are waiting
		close(block)
		wg.Wait()

		// When multiple goroutines are waiting together, they should all see shared=true
		// because dups > 1 for all of them
		for i, shared := range results {
			if !shared {
				t.Errorf("goroutine %d: shared=%v; want true (multiple goroutines waiting)", i, shared)
			}
		}
	})

	// Test 2: Sequential calls should report shared=false
	t.Run("sequential_calls", func(t *testing.T) {
		var calls atomic.Int32

		// First call
		_, shared1, err := g.Do("key2", func() (int, error) {
			return int(calls.Add(1)), nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		// First call has no other goroutines waiting, might be false
		t.Logf("first call shared=%v", shared1)

		// Wait for first call to complete
		time.Sleep(10 * time.Millisecond)

		// Second call (after first completes) should also not be shared
		_, shared2, err := g.Do("key2", func() (int, error) {
			return int(calls.Add(1)), nil
		})
		if err != nil {
			t.Errorf("Do error: %v", err)
		}
		t.Logf("second call shared=%v", shared2)

		// Since calls are sequential and the first completes before the second starts,
		// at least one should be marked as not shared
		if shared1 && shared2 {
			t.Error("expected at least one sequential call to not be shared")
		}
	})
}
