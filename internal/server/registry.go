package server

import (
	"net"
	"sync"

	"github.com/hashicorp/yamux"
)

// ClientSlot represents a client's port allocation and associated resources
type ClientSlot struct {
	// Immutable after creation
	port       int
	clientName string
	clientID   string // UUID for logging

	// Mutable (protected by Registry mutex)
	session       *yamux.Session
	socksListener net.Listener

	// Cleanup coordination
	stopOnce sync.Once
	stopFunc func()
}

// Registry manages port allocations and client sessions
type Registry struct {
	mu    sync.RWMutex
	slots map[int]*ClientSlot
}

// NewRegistry creates a new Registry instance
func NewRegistry() *Registry {
	return &Registry{
		slots: make(map[int]*ClientSlot),
	}
}

// ReservePorts atomically reserves the specified ports.
// Returns a release function that can be called to release the reservation,
// or an error if any port is already reserved.
// All ports must be available or none are reserved (atomic operation).
func (r *Registry) ReservePorts(ports []int) (releaseFunc func(), err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check all ports are available atomically
	for _, port := range ports {
		if _, exists := r.slots[port]; exists {
			return nil, &PortInUseError{Port: port}
		}
	}

	// Reserve all ports with placeholder slots
	for _, port := range ports {
		r.slots[port] = &ClientSlot{
			port: port,
		}
	}

	// Return release function for cleanup
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

// PortInUseError indicates a port is already reserved
type PortInUseError struct {
	Port int
}

func (e *PortInUseError) Error() string {
	return "port already in use"
}

// ClientMeta contains metadata about a client
type ClientMeta struct {
	ClientName string
	ClientID   string
}

// BindSession associates a yamux session and SOCKS listener with a reserved port.
// The port must have been previously reserved via ReservePorts.
func (r *Registry) BindSession(port int, sess *yamux.Session, listener net.Listener, meta ClientMeta) error {
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

	return nil
}

// PortNotReservedError indicates a port was not reserved
type PortNotReservedError struct {
	Port int
}

func (e *PortNotReservedError) Error() string {
	return "port not reserved"
}

// GetSession retrieves the yamux session associated with a port.
// Returns the session and true if found, nil and false otherwise.
// Thread-safe for concurrent reads.
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

		// Use stopOnce to ensure cleanup happens only once
		slot.stopOnce.Do(func() {
			// Close SOCKS listener if it exists
			if slot.socksListener != nil {
				slot.socksListener.Close()
			}

			// Execute custom stop function if provided
			if slot.stopFunc != nil {
				slot.stopFunc()
			}
		})

		// Remove from registry
		delete(r.slots, port)
	}
}
