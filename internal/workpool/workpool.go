package workpool

import (
	"runtime"
	"sync"
	"sync/atomic"
)

var configuredWorkers atomic.Int32

// SetWorkers configures the maximum worker goroutines used by Run.
// Values <= 0 reset to automatic (all CPU cores via runtime.GOMAXPROCS(0)).
func SetWorkers(n int) {
	if n < 0 {
		n = 0
	}
	if n > 64 {
		n = 64
	}
	configuredWorkers.Store(int32(n))
}

// Workers returns the active concurrency limit used by Run.
func Workers() int {
	if n := int(configuredWorkers.Load()); n > 0 {
		return n
	}
	return runtime.GOMAXPROCS(0)
}

// Run calls fn for every index in [0, count) on all cores (or configured
// worker threads). Indexes run in any order; fn must be safe for concurrent
// use. It stops at the first error and returns it.
func Run(count int, fn func(index int) error) error {
	var next atomic.Int64
	var failed atomic.Bool
	var firstErr error
	var errOnce sync.Once
	var wg sync.WaitGroup
	for range min(count, Workers()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !failed.Load() {
				index := int(next.Add(1) - 1)
				if index >= count {
					return
				}
				if err := fn(index); err != nil {
					errOnce.Do(func() { firstErr = err })
					failed.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}
