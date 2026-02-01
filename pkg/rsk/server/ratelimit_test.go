package server

import (
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5, 5*time.Minute)
	defer rl.Close()

	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
	if rl.maxFailures != 5 {
		t.Errorf("Expected maxFailures=5, got %d", rl.maxFailures)
	}
	if rl.blockDuration != 5*time.Minute {
		t.Errorf("Expected blockDuration=5m, got %v", rl.blockDuration)
	}
	if rl.limiters == nil {
		t.Error("limiters map not initialized")
	}
}

func TestRecordFailure(t *testing.T) {
	rl := NewRateLimiter(3, 5*time.Minute)
	defer rl.Close()

	ip := "192.168.1.1"

	// First two failures should not trigger block
	if blocked := rl.RecordFailure(ip); blocked {
		t.Error("First failure should not trigger block")
	}
	if blocked := rl.RecordFailure(ip); blocked {
		t.Error("Second failure should not trigger block")
	}

	// Third failure should trigger block
	if blocked := rl.RecordFailure(ip); !blocked {
		t.Error("Third failure should trigger block")
	}

	// Verify the record exists
	rl.mu.RLock()
	entry, exists := rl.limiters[ip]
	rl.mu.RUnlock()

	if !exists {
		t.Fatal("Failure record not created")
	}
	if entry.failures != 3 {
		t.Errorf("Expected failures=3, got %d", entry.failures)
	}
	if entry.blockedAt.IsZero() {
		t.Error("blockedAt should be set after threshold reached")
	}
}

func TestIsBlocked(t *testing.T) {
	rl := NewRateLimiter(2, 100*time.Millisecond)
	defer rl.Close()

	ip := "192.168.1.1"

	// IP not blocked initially
	if rl.IsBlocked(ip) {
		t.Error("IP should not be blocked initially")
	}

	// Record failures to trigger block
	rl.RecordFailure(ip)
	rl.RecordFailure(ip)

	// IP should be blocked now
	if !rl.IsBlocked(ip) {
		t.Error("IP should be blocked after threshold")
	}

	// Wait for block to expire
	time.Sleep(150 * time.Millisecond)

	// IP should no longer be blocked
	if rl.IsBlocked(ip) {
		t.Error("IP should not be blocked after expiration")
	}
}

func TestReset(t *testing.T) {
	rl := NewRateLimiter(3, 5*time.Minute)
	defer rl.Close()

	ip := "192.168.1.1"

	// Record some failures
	rl.RecordFailure(ip)
	rl.RecordFailure(ip)

	// Verify record exists
	rl.mu.RLock()
	_, exists := rl.limiters[ip]
	rl.mu.RUnlock()
	if !exists {
		t.Fatal("Failure record should exist")
	}

	// Reset the IP
	rl.Reset(ip)

	// Verify record removed
	rl.mu.RLock()
	_, exists = rl.limiters[ip]
	rl.mu.RUnlock()
	if exists {
		t.Error("Failure record should be removed after reset")
	}

	// IP should not be blocked
	if rl.IsBlocked(ip) {
		t.Error("IP should not be blocked after reset")
	}
}

func TestCleanup(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)
	defer rl.Close()

	ip := "192.168.1.1"

	// Trigger block
	rl.RecordFailure(ip)
	rl.RecordFailure(ip)

	// Verify record exists
	rl.mu.RLock()
	_, exists := rl.limiters[ip]
	rl.mu.RUnlock()
	if !exists {
		t.Fatal("Failure record should exist")
	}

	// Wait for cleanup period (2 * blockDuration)
	time.Sleep(150 * time.Millisecond)

	// Manually trigger cleanup
	rl.cleanup()

	// Verify record removed
	rl.mu.RLock()
	_, exists = rl.limiters[ip]
	rl.mu.RUnlock()
	if exists {
		t.Error("Failure record should be cleaned up")
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(10, 5*time.Minute)
	defer rl.Close()

	var wg sync.WaitGroup
	numGoroutines := 50
	numOperations := 100

	// Concurrent RecordFailure
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := "192.168.1.1"
			for j := 0; j < numOperations; j++ {
				rl.RecordFailure(ip)
			}
		}(i)
	}

	// Concurrent IsBlocked
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := "192.168.1.1"
			for j := 0; j < numOperations; j++ {
				rl.IsBlocked(ip)
			}
		}(i)
	}

	// Concurrent Reset
	for i := 0; i < numGoroutines/10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ip := "192.168.1.1"
			for j := 0; j < numOperations/10; j++ {
				rl.Reset(ip)
			}
		}(i)
	}

	wg.Wait()

	// Test should complete without panics or deadlocks
}

func TestMultipleIPs(t *testing.T) {
	rl := NewRateLimiter(2, 5*time.Minute)
	defer rl.Close()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Block ip1
	rl.RecordFailure(ip1)
	rl.RecordFailure(ip1)

	// ip1 should be blocked
	if !rl.IsBlocked(ip1) {
		t.Error("ip1 should be blocked")
	}

	// ip2 should not be blocked
	if rl.IsBlocked(ip2) {
		t.Error("ip2 should not be blocked")
	}

	// Record one failure for ip2
	rl.RecordFailure(ip2)

	// ip2 should still not be blocked
	if rl.IsBlocked(ip2) {
		t.Error("ip2 should not be blocked after one failure")
	}

	// ip1 should still be blocked
	if !rl.IsBlocked(ip1) {
		t.Error("ip1 should still be blocked")
	}
}

func TestClose(t *testing.T) {
	rl := NewRateLimiter(5, 5*time.Minute)

	// Close should not panic
	rl.Close()

	// Verify cleanup goroutine stopped (ticker should be stopped)
	// We can't directly test goroutine termination, but Close should complete quickly
	done := make(chan struct{})
	go func() {
		rl.Close() // Second close should also not panic
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Close() did not complete in reasonable time")
	}
}

func TestBlockExpiration(t *testing.T) {
	rl := NewRateLimiter(1, 100*time.Millisecond)
	defer rl.Close()

	ip := "192.168.1.1"

	// Trigger block
	rl.RecordFailure(ip)

	// Should be blocked immediately
	if !rl.IsBlocked(ip) {
		t.Error("IP should be blocked immediately after threshold")
	}

	// Wait half the block duration
	time.Sleep(50 * time.Millisecond)

	// Should still be blocked
	if !rl.IsBlocked(ip) {
		t.Error("IP should still be blocked")
	}

	// Wait for full block duration to expire
	time.Sleep(60 * time.Millisecond)

	// Should no longer be blocked
	if rl.IsBlocked(ip) {
		t.Error("IP should not be blocked after expiration")
	}
}
