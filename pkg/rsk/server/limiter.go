package server

import (
	"golang.org/x/sync/semaphore"
)

// ConnectionLimiter enforces a maximum number of concurrent connections.
type ConnectionLimiter struct {
	sem      *semaphore.Weighted
	maxConns int64
}

// NewConnectionLimiter creates a new connection limiter.
func NewConnectionLimiter(maxConns int) *ConnectionLimiter {
	return &ConnectionLimiter{
		sem:      semaphore.NewWeighted(int64(maxConns)),
		maxConns: int64(maxConns),
	}
}

// Acquire attempts to acquire a connection slot (non-blocking).
func (cl *ConnectionLimiter) Acquire() bool {
	return cl.sem.TryAcquire(1)
}

// Release releases a connection slot.
func (cl *ConnectionLimiter) Release() {
	cl.sem.Release(1)
}

// Available returns the number of available connection slots.
func (cl *ConnectionLimiter) Available() int {
	acquired := int64(0)
	for i := int64(0); i < cl.maxConns; i++ {
		if cl.sem.TryAcquire(1) {
			acquired++
		} else {
			break
		}
	}

	if acquired > 0 {
		cl.sem.Release(acquired)
	}

	return int(acquired)
}
