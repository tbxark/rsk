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
	err = r.BindSession(port, mockSession, mockListener, meta)
	require.NoError(t, err)

	// Verify binding
	slot := r.slots[port]
	assert.Equal(t, mockSession, slot.session)
	assert.Equal(t, mockListener, slot.socksListener)
	assert.Equal(t, "test-client", slot.clientName)
	assert.Equal(t, "test-id-123", slot.clientID)
}

func TestBindSession_PortNotReserved(t *testing.T) {
	r := NewRegistry()
	port := 20001

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	// Try to bind without reserving
	err := r.BindSession(port, mockSession, mockListener, meta)
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

	err = r.BindSession(port, mockSession, mockListener, meta)
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
		err = r.BindSession(port, mockSession, mockListener, meta)
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

	err = r.BindSession(ports[0], mockSession, mockListener, meta)
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
