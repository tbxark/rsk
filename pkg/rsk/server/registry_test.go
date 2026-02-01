package server

import (
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	assert.NotNil(t, r)
	assert.NotNil(t, r.slots)
}

func TestReservePorts_Success(t *testing.T) {
	r := NewRegistry()
	ports := []int{20001, 20002, 20003}

	release, err := r.ReservePorts(ports)
	require.NoError(t, err)
	require.NotNil(t, release)

	// Verify ports are reserved
	for _, port := range ports {
		_, exists := r.slots[port]
		assert.True(t, exists, "port %d should be reserved", port)
	}

	// Release ports
	release()

	// Verify ports are released
	for _, port := range ports {
		_, exists := r.slots[port]
		assert.False(t, exists, "port %d should be released", port)
	}
}

func TestReservePorts_PortInUse(t *testing.T) {
	r := NewRegistry()

	// Reserve first set of ports
	ports1 := []int{20001, 20002}
	release1, err := r.ReservePorts(ports1)
	require.NoError(t, err)
	defer release1()

	// Try to reserve overlapping ports
	ports2 := []int{20002, 20003}
	release2, err := r.ReservePorts(ports2)
	assert.Error(t, err)
	assert.Nil(t, release2)
	assert.IsType(t, &PortInUseError{}, err)

	// Verify port 20003 was not reserved (atomic failure)
	_, exists := r.slots[20003]
	assert.False(t, exists, "port 20003 should not be reserved due to atomic failure")
}

func TestReservePorts_ReleaseIdempotent(t *testing.T) {
	r := NewRegistry()
	ports := []int{20001}

	release, err := r.ReservePorts(ports)
	require.NoError(t, err)

	// Call release multiple times
	release()
	release()
	release()

	// Should not panic or cause issues
	_, exists := r.slots[20001]
	assert.False(t, exists)
}

func TestBindSession(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve port first
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	// Create mock session and listener
	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}

	meta := ClientMeta{
		ClientName: "test-client",
		ClientID:   "test-id-123",
	}

	// Bind session
	err = r.BindSession(port, mockSession, mockListener, meta, 100)
	require.NoError(t, err)

	// Verify binding
	slot := r.slots[port]
	assert.Equal(t, mockSession, slot.session)
	assert.Equal(t, mockListener, slot.socksListener)
	assert.Equal(t, "test-client", slot.clientName)
	assert.Equal(t, "test-id-123", slot.clientID)
	assert.Equal(t, int32(100), slot.maxConns)
	assert.Equal(t, int32(0), slot.activeConns)
}

func TestBindSession_PortNotReserved(t *testing.T) {
	r := NewRegistry()
	port := 20001

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	// Try to bind without reserving
	err := r.BindSession(port, mockSession, mockListener, meta, 100)
	assert.Error(t, err)
	assert.IsType(t, &PortNotReservedError{}, err)
}

func TestGetSession(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 100)
	require.NoError(t, err)

	// Get session
	sess, found := r.GetSession(port)
	assert.True(t, found)
	assert.Equal(t, mockSession, sess)
}

func TestGetSession_NotFound(t *testing.T) {
	r := NewRegistry()

	sess, found := r.GetSession(20001)
	assert.False(t, found)
	assert.Nil(t, sess)
}

func TestGetSession_NoSessionBound(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve but don't bind
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	sess, found := r.GetSession(port)
	assert.False(t, found)
	assert.Nil(t, sess)
}

func TestReleasePorts(t *testing.T) {
	r := NewRegistry()
	ports := []int{20001, 20002}

	// Reserve and bind
	release, err := r.ReservePorts(ports)
	require.NoError(t, err)
	defer release()

	mockListener := &mockNetListener{}
	mockSession := &yamux.Session{}
	meta := ClientMeta{ClientName: "test"}

	for _, port := range ports {
		err = r.BindSession(port, mockSession, mockListener, meta, 100)
		require.NoError(t, err)
	}

	// Release ports
	r.ReleasePorts(ports)

	// Verify ports are removed
	for _, port := range ports {
		_, exists := r.slots[port]
		assert.False(t, exists)
	}

	// Verify listener was closed
	assert.True(t, mockListener.closed)
}

func TestReleasePorts_Idempotent(t *testing.T) {
	r := NewRegistry()
	ports := []int{20001}

	release, err := r.ReservePorts(ports)
	require.NoError(t, err)
	defer release()

	mockListener := &mockNetListener{}
	mockSession := &yamux.Session{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(ports[0], mockSession, mockListener, meta, 100)
	require.NoError(t, err)

	// Call ReleasePorts multiple times
	r.ReleasePorts(ports)
	r.ReleasePorts(ports)
	r.ReleasePorts(ports)

	// Should not panic
	// Listener should be closed only once
	assert.True(t, mockListener.closed)
	assert.Equal(t, 1, mockListener.closeCount)
}

func TestReleasePorts_NonExistentPort(t *testing.T) {
	r := NewRegistry()

	// Should not panic when releasing non-existent ports
	r.ReleasePorts([]int{20001, 20002})
}

// Mock net.Listener for testing
type mockNetListener struct {
	closed     bool
	closeCount int
}

func (m *mockNetListener) Accept() (net.Conn, error) {
	return nil, nil
}

func (m *mockNetListener) Close() error {
	m.closed = true
	m.closeCount++
	return nil
}

func (m *mockNetListener) Addr() net.Addr {
	return nil
}

func TestIncrementConnections_Success(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind with maxConns=3
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 3)
	require.NoError(t, err)

	// Increment connections
	assert.True(t, r.IncrementConnections(port))
	assert.Equal(t, 1, r.GetConnectionCount(port))

	assert.True(t, r.IncrementConnections(port))
	assert.Equal(t, 2, r.GetConnectionCount(port))

	assert.True(t, r.IncrementConnections(port))
	assert.Equal(t, 3, r.GetConnectionCount(port))
}

func TestIncrementConnections_LimitReached(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind with maxConns=2
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 2)
	require.NoError(t, err)

	// Increment to limit
	assert.True(t, r.IncrementConnections(port))
	assert.True(t, r.IncrementConnections(port))

	// Try to exceed limit
	assert.False(t, r.IncrementConnections(port))
	assert.Equal(t, 2, r.GetConnectionCount(port))
}

func TestIncrementConnections_PortNotFound(t *testing.T) {
	r := NewRegistry()

	// Try to increment on non-existent port
	assert.False(t, r.IncrementConnections(20001))
}

func TestDecrementConnections(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 10)
	require.NoError(t, err)

	// Increment then decrement
	r.IncrementConnections(port)
	r.IncrementConnections(port)
	r.IncrementConnections(port)
	assert.Equal(t, 3, r.GetConnectionCount(port))

	r.DecrementConnections(port)
	assert.Equal(t, 2, r.GetConnectionCount(port))

	r.DecrementConnections(port)
	assert.Equal(t, 1, r.GetConnectionCount(port))

	r.DecrementConnections(port)
	assert.Equal(t, 0, r.GetConnectionCount(port))
}

func TestDecrementConnections_PortNotFound(t *testing.T) {
	r := NewRegistry()

	// Should not panic when decrementing non-existent port
	r.DecrementConnections(20001)
}

func TestGetConnectionCount_PortNotFound(t *testing.T) {
	r := NewRegistry()

	count := r.GetConnectionCount(20001)
	assert.Equal(t, 0, count)
}

func TestConnectionCounting_Concurrent(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind with high limit
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 1000)
	require.NoError(t, err)

	// Concurrently increment and decrement
	const numGoroutines = 100
	const incrementsPerGoroutine = 10

	done := make(chan bool, numGoroutines)

	// Increment goroutines
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			for j := 0; j < incrementsPerGoroutine; j++ {
				r.IncrementConnections(port)
			}
			done <- true
		}()
	}

	// Decrement goroutines
	for i := 0; i < numGoroutines/2; i++ {
		go func() {
			for j := 0; j < incrementsPerGoroutine; j++ {
				r.DecrementConnections(port)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Final count should be 0 (equal increments and decrements)
	count := r.GetConnectionCount(port)
	assert.Equal(t, 0, count)
}

func TestConnectionCounting_LimitEnforcement(t *testing.T) {
	r := NewRegistry()
	port := 20001

	// Reserve and bind with limit of 5
	release, err := r.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = r.BindSession(port, mockSession, mockListener, meta, 5)
	require.NoError(t, err)

	// Try to increment beyond limit concurrently
	const numGoroutines = 20
	successCount := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			successCount <- r.IncrementConnections(port)
		}()
	}

	// Count successful increments
	successes := 0
	for i := 0; i < numGoroutines; i++ {
		if <-successCount {
			successes++
		}
	}

	// Should have exactly 5 successes (the limit)
	assert.Equal(t, 5, successes)
	assert.Equal(t, 5, r.GetConnectionCount(port))
}
