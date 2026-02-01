package server

import (
	"sync"
	"testing"
)

func TestNewConnectionLimiter(t *testing.T) {
	maxConns := 10
	limiter := NewConnectionLimiter(maxConns)

	if limiter == nil {
		t.Fatal("NewConnectionLimiter returned nil")
	}

	if limiter.maxConns != maxConns {
		t.Errorf("Expected maxConns=%d, got %d", maxConns, limiter.maxConns)
	}

	if limiter.Available() != maxConns {
		t.Errorf("Expected %d available connections, got %d", maxConns, limiter.Available())
	}
}

func TestAcquireReleaseCycle(t *testing.T) {
	limiter := NewConnectionLimiter(5)

	// Acquire a slot
	if !limiter.Acquire() {
		t.Fatal("Failed to acquire slot when limiter is empty")
	}

	if limiter.Available() != 4 {
		t.Errorf("Expected 4 available slots, got %d", limiter.Available())
	}

	// Release the slot
	limiter.Release()

	if limiter.Available() != 5 {
		t.Errorf("Expected 5 available slots after release, got %d", limiter.Available())
	}
}

func TestLimitEnforcement(t *testing.T) {
	maxConns := 3
	limiter := NewConnectionLimiter(maxConns)

	// Acquire up to the limit
	for i := 0; i < maxConns; i++ {
		if !limiter.Acquire() {
			t.Fatalf("Failed to acquire slot %d/%d", i+1, maxConns)
		}
	}

	if limiter.Available() != 0 {
		t.Errorf("Expected 0 available slots, got %d", limiter.Available())
	}

	// Try to acquire beyond the limit
	if limiter.Acquire() {
		t.Error("Acquire succeeded when limit was reached")
	}

	// Release one slot
	limiter.Release()

	if limiter.Available() != 1 {
		t.Errorf("Expected 1 available slot after release, got %d", limiter.Available())
	}

	// Should be able to acquire again
	if !limiter.Acquire() {
		t.Error("Failed to acquire after releasing a slot")
	}
}

func TestConcurrentAccess(t *testing.T) {
	maxConns := 10
	limiter := NewConnectionLimiter(maxConns)
	numGoroutines := 50

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Launch many goroutines trying to acquire
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Acquire() {
				mu.Lock()
				successCount++
				mu.Unlock()
				// Simulate some work
				limiter.Release()
			}
		}()
	}

	wg.Wait()

	// All goroutines should have succeeded since they release
	if successCount != numGoroutines {
		t.Errorf("Expected %d successful acquisitions, got %d", numGoroutines, successCount)
	}

	// All slots should be available again
	if limiter.Available() != maxConns {
		t.Errorf("Expected %d available slots after all releases, got %d", maxConns, limiter.Available())
	}
}

func TestReleaseWithoutAcquire(t *testing.T) {
	limiter := NewConnectionLimiter(5)

	// Release without acquiring should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Release without acquire caused panic: %v", r)
		}
	}()

	limiter.Release()

	// Available should not exceed maxConns
	if limiter.Available() > limiter.maxConns {
		t.Errorf("Available (%d) exceeds maxConns (%d)", limiter.Available(), limiter.maxConns)
	}
}

func TestConcurrentAcquireRelease(t *testing.T) {
	maxConns := 5
	limiter := NewConnectionLimiter(maxConns)
	iterations := 100

	var wg sync.WaitGroup

	// Multiple goroutines acquiring and releasing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if limiter.Acquire() {
					limiter.Release()
				}
			}
		}()
	}

	wg.Wait()

	// All slots should be available
	if limiter.Available() != maxConns {
		t.Errorf("Expected %d available slots, got %d", maxConns, limiter.Available())
	}
}
