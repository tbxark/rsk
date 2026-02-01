package server

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter tracks rate limiters per IP address for authentication attempts.
type IPRateLimiter struct {
	mu            sync.RWMutex
	limiters      map[string]*ipLimiterEntry
	maxFailures   int
	blockDuration time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	closeOnce     sync.Once
}

type ipLimiterEntry struct {
	limiter   *rate.Limiter
	failures  int
	blockedAt time.Time
}

// NewRateLimiter creates a new IP-based rate limiter.
func NewRateLimiter(maxFailures int, blockDuration time.Duration) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters:      make(map[string]*ipLimiterEntry),
		maxFailures:   maxFailures,
		blockDuration: blockDuration,
		cleanupTicker: time.NewTicker(1 * time.Minute),
		stopCleanup:   make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// RecordFailure records an authentication failure. Returns true if IP should be blocked.
func (rl *IPRateLimiter) RecordFailure(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.limiters[ip]
	if !exists {
		entry = &ipLimiterEntry{
			limiter:  rate.NewLimiter(rate.Every(time.Second), rl.maxFailures),
			failures: 0,
		}
		rl.limiters[ip] = entry
	}

	entry.failures++

	if entry.failures >= rl.maxFailures {
		entry.blockedAt = time.Now()
		return true
	}

	return false
}

// IsBlocked checks if the given IP is currently blocked.
func (rl *IPRateLimiter) IsBlocked(ip string) bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	entry, exists := rl.limiters[ip]
	if !exists {
		return false
	}

	if !entry.blockedAt.IsZero() {
		if time.Since(entry.blockedAt) < rl.blockDuration {
			return true
		}
	}

	return false
}

// Reset clears the failure record for the given IP.
func (rl *IPRateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.limiters, ip)
}

func (rl *IPRateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

func (rl *IPRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, entry := range rl.limiters {
		if !entry.blockedAt.IsZero() && now.Sub(entry.blockedAt) > rl.blockDuration*2 {
			delete(rl.limiters, ip)
		}
	}
}

// Close stops the cleanup goroutine.
func (rl *IPRateLimiter) Close() {
	rl.closeOnce.Do(func() {
		close(rl.stopCleanup)
		rl.cleanupTicker.Stop()
	})
}
