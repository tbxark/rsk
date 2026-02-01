package server

import (
	"sync"
	"time"
)

// failureRecord tracks authentication failures for an IP address
type failureRecord struct {
	count     int
	blockedAt time.Time
}

// RateLimiter tracks and limits authentication failures per IP address
type RateLimiter struct {
	mu            sync.RWMutex
	failures      map[string]*failureRecord
	maxFailures   int
	blockDuration time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	closeOnce     sync.Once
}

// NewRateLimiter creates a new rate limiter with the specified parameters
func NewRateLimiter(maxFailures int, blockDuration time.Duration) *RateLimiter {
	rl := &RateLimiter{
		failures:      make(map[string]*failureRecord),
		maxFailures:   maxFailures,
		blockDuration: blockDuration,
		cleanupTicker: time.NewTicker(1 * time.Minute),
		stopCleanup:   make(chan struct{}),
	}

	// Start background cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// RecordFailure records an authentication failure for the given IP
// Returns true if the IP should be blocked
func (rl *RateLimiter) RecordFailure(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	record, exists := rl.failures[ip]
	if !exists {
		record = &failureRecord{count: 0}
		rl.failures[ip] = record
	}

	record.count++

	// Check if threshold reached
	if record.count >= rl.maxFailures {
		record.blockedAt = time.Now()
		return true
	}

	return false
}

// IsBlocked checks if the given IP is currently blocked
func (rl *RateLimiter) IsBlocked(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	record, exists := rl.failures[ip]
	if !exists {
		return false
	}

	// Check if block has expired
	if !record.blockedAt.IsZero() {
		if time.Since(record.blockedAt) < rl.blockDuration {
			return true
		}
	}

	return false
}

// Reset clears the failure record for the given IP
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.failures, ip)
}

// cleanupLoop periodically removes expired entries
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries from the failures map
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, record := range rl.failures {
		// Remove entries where block expired more than blockDuration ago
		if !record.blockedAt.IsZero() && now.Sub(record.blockedAt) > rl.blockDuration*2 {
			delete(rl.failures, ip)
		}
	}
}

// Close stops the cleanup goroutine and releases resources
func (rl *RateLimiter) Close() {
	rl.closeOnce.Do(func() {
		close(rl.stopCleanup)
		rl.cleanupTicker.Stop()
	})
}
