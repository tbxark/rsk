package server

// ConnectionLimiter enforces a maximum number of concurrent connections
// using a semaphore pattern with a buffered channel.
type ConnectionLimiter struct {
	semaphore chan struct{}
	maxConns  int
}

// NewConnectionLimiter creates a new connection limiter with the specified
// maximum number of concurrent connections.
func NewConnectionLimiter(maxConns int) *ConnectionLimiter {
	return &ConnectionLimiter{
		semaphore: make(chan struct{}, maxConns),
		maxConns:  maxConns,
	}
}

// Acquire attempts to acquire a connection slot. Returns true if successful,
// false if the limit has been reached. This is a non-blocking operation.
func (cl *ConnectionLimiter) Acquire() bool {
	select {
	case cl.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases a connection slot, making it available for new connections.
// This should be called in a defer statement to ensure slots are always released.
func (cl *ConnectionLimiter) Release() {
	select {
	case <-cl.semaphore:
	default:
		// Should not happen in normal operation, but prevents panic
	}
}

// Available returns the number of available connection slots.
// This is useful for metrics and monitoring.
func (cl *ConnectionLimiter) Available() int {
	return cl.maxConns - len(cl.semaphore)
}
