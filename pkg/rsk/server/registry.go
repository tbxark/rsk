package server

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
)

type ClientSlot struct {
	port       int    // Port number
	clientName string // Client name
	clientID   string // Client UUID

	session       *yamux.Session // Yamux session
	socksListener net.Listener   // SOCKS5 listener

	activeConns int32 // Active SOCKS5 connections (atomic)
	maxConns    int32 // Maximum allowed connections

	stopOnce sync.Once // Ensures cleanup happens once
	stopFunc func()    // Custom cleanup function
}

type Registry struct {
	mu    sync.RWMutex        // Protects slots
	slots map[int]*ClientSlot // Port to client slot mapping
}

// NewRegistry creates a new Registry.
func NewRegistry() *Registry {
	return &Registry{
		slots: make(map[int]*ClientSlot),
	}
}

// ReservePorts atomically reserves the specified ports.
func (r *Registry) ReservePorts(ports []int) (releaseFunc func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, port := range ports {
		if _, exists := r.slots[port]; exists {
			return nil, &PortInUseError{Port: port}
		}
	}

	for _, port := range ports {
		r.slots[port] = &ClientSlot{
			port: port,
		}
	}

	released := false
	releaseFunc = func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if released {
			return
		}
		released = true

		for _, port := range ports {
			delete(r.slots, port)
		}
	}

	return releaseFunc, nil
}

type PortInUseError struct {
	Port int
}

func (e *PortInUseError) Error() string {
	return "port already in use"
}

type ClientMeta struct {
	ClientName string // Client name
	ClientID   string // Client UUID
}

// BindSession associates a yamux session and SOCKS listener with a reserved port.
func (r *Registry) BindSession(port int, sess *yamux.Session, listener net.Listener, meta ClientMeta, maxConns int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	slot, exists := r.slots[port]
	if !exists {
		return &PortNotReservedError{Port: port}
	}

	// Update the slot with session and listener
	slot.session = sess
	slot.socksListener = listener
	slot.clientName = meta.ClientName
	slot.clientID = meta.ClientID
	slot.maxConns = maxConns
	slot.activeConns = 0

	return nil
}

type PortNotReservedError struct {
	Port int
}

func (e *PortNotReservedError) Error() string {
	return "port not reserved"
}

// GetSession retrieves the yamux session associated with a port.
func (r *Registry) GetSession(port int) (*yamux.Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	slot, exists := r.slots[port]
	if !exists || slot.session == nil {
		return nil, false
	}

	return slot.session, true
}

// ReleasePorts removes the specified ports from the registry and closes associated resources.
// This operation is idempotent - calling it multiple times is safe.
func (r *Registry) ReleasePorts(ports []int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, port := range ports {
		slot, exists := r.slots[port]
		if !exists {
			continue
		}

		slot.stopOnce.Do(func() {
			if slot.socksListener != nil {
				_ = slot.socksListener.Close()
			}

			if slot.stopFunc != nil {
				slot.stopFunc()
			}
		})

		delete(r.slots, port)
	}
}

// IncrementConnections atomically increments the connection count for a port.
// Returns true if the increment was successful, false if the limit was reached.
func (r *Registry) IncrementConnections(port int) bool {
	r.mu.RLock()
	slot, exists := r.slots[port]
	r.mu.RUnlock()

	if !exists {
		return false
	}

	// Atomically check and increment
	for {
		current := atomic.LoadInt32(&slot.activeConns)
		if current >= slot.maxConns {
			return false
		}
		if atomic.CompareAndSwapInt32(&slot.activeConns, current, current+1) {
			return true
		}
	}
}

// DecrementConnections atomically decrements the connection count for a port.
func (r *Registry) DecrementConnections(port int) {
	r.mu.RLock()
	slot, exists := r.slots[port]
	r.mu.RUnlock()

	if !exists {
		return
	}

	atomic.AddInt32(&slot.activeConns, -1)
}

// GetConnectionCount returns the current connection count for a port.
func (r *Registry) GetConnectionCount(port int) int {
	r.mu.RLock()
	slot, exists := r.slots[port]
	r.mu.RUnlock()

	if !exists {
		return 0
	}

	return int(atomic.LoadInt32(&slot.activeConns))
}
