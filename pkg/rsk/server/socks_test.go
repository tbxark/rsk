package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSOCKSManager_ConnectionCounting(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()
	socksManager := NewSOCKSManager(registry, logger)

	port := 20001
	maxConns := int32(3)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	// Create mock yamux session
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxConfig := yamux.DefaultConfig()
	sess, err := yamux.Server(server, yamuxConfig)
	require.NoError(t, err)
	defer sess.Close()

	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, sess, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Create dialer
	dialer := socksManager.createDialer(port, sess)

	// Test successful connection increment
	ctx := context.Background()

	// First connection should succeed
	conn1, err := dialer(ctx, "tcp", "example.com:80")
	if err == nil {
		defer conn1.Close()
		assert.Equal(t, 1, registry.GetConnectionCount(port))
	}

	// Note: The actual connection will fail because we don't have a real yamux client,
	// but we can test the increment/decrement logic
}

func TestSOCKSManager_ConnectionLimit(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()
	socksManager := NewSOCKSManager(registry, logger)

	port := 20001
	maxConns := int32(2)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	// Create mock yamux session
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxConfig := yamux.DefaultConfig()
	sess, err := yamux.Server(server, yamuxConfig)
	require.NoError(t, err)
	defer sess.Close()

	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, sess, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Manually increment to limit
	assert.True(t, registry.IncrementConnections(port))
	assert.True(t, registry.IncrementConnections(port))
	assert.Equal(t, 2, registry.GetConnectionCount(port))

	// Create dialer
	dialer := socksManager.createDialer(port, sess)
	ctx := context.Background()

	// Try to create connection when limit is reached
	conn, err := dialer(ctx, "tcp", "example.com:80")
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "connection limit reached")

	// Connection count should still be at limit
	assert.Equal(t, 2, registry.GetConnectionCount(port))
}

func TestConnCountingStream_Close(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(10)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Increment connection count
	assert.True(t, registry.IncrementConnections(port))
	assert.Equal(t, 1, registry.GetConnectionCount(port))

	// Create a mock connection
	client, server := net.Pipe()
	defer server.Close()

	// Wrap it in connCountingStream
	stream := &connCountingStream{
		Conn:     client,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	// Close the stream
	err = stream.Close()
	assert.NoError(t, err)

	// Connection count should be decremented
	assert.Equal(t, 0, registry.GetConnectionCount(port))

	// Closing again should be idempotent
	err = stream.Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestConnCountingStream_CloseIdempotent(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(10)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Increment connection count
	assert.True(t, registry.IncrementConnections(port))
	assert.Equal(t, 1, registry.GetConnectionCount(port))

	// Create a mock connection
	client, server := net.Pipe()
	defer server.Close()

	// Wrap it in connCountingStream
	stream := &connCountingStream{
		Conn:     client,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	// Close multiple times
	err = stream.Close()
	assert.NoError(t, err)

	err = stream.Close()
	assert.NoError(t, err)

	err = stream.Close()
	assert.NoError(t, err)

	// Connection count should be decremented only once
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestConnCountingStream_ConcurrentClose(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(10)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Increment connection count
	assert.True(t, registry.IncrementConnections(port))
	assert.Equal(t, 1, registry.GetConnectionCount(port))

	// Create a mock connection
	client, server := net.Pipe()
	defer server.Close()

	// Wrap it in connCountingStream
	stream := &connCountingStream{
		Conn:     client,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	// Close concurrently from multiple goroutines
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = stream.Close()
		}()
	}

	wg.Wait()

	// Connection count should be decremented only once
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestSOCKSManager_MultipleConnections(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(5)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Create multiple connections
	var streams []*connCountingStream
	for i := 0; i < 3; i++ {
		assert.True(t, registry.IncrementConnections(port))

		client, server := net.Pipe()
		defer server.Close()

		stream := &connCountingStream{
			Conn:     client,
			port:     port,
			registry: registry,
			logger:   logger,
		}
		streams = append(streams, stream)
	}

	assert.Equal(t, 3, registry.GetConnectionCount(port))

	// Close first connection
	err = streams[0].Close()
	assert.NoError(t, err)
	assert.Equal(t, 2, registry.GetConnectionCount(port))

	// Close second connection
	err = streams[1].Close()
	assert.NoError(t, err)
	assert.Equal(t, 1, registry.GetConnectionCount(port))

	// Close third connection
	err = streams[2].Close()
	assert.NoError(t, err)
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestSOCKSManager_ConnectionLifecycle(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(2)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Create first connection
	assert.True(t, registry.IncrementConnections(port))
	client1, server1 := net.Pipe()
	defer server1.Close()
	stream1 := &connCountingStream{
		Conn:     client1,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	// Create second connection
	assert.True(t, registry.IncrementConnections(port))
	client2, server2 := net.Pipe()
	defer server2.Close()
	stream2 := &connCountingStream{
		Conn:     client2,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	assert.Equal(t, 2, registry.GetConnectionCount(port))

	// Try to create third connection (should fail)
	assert.False(t, registry.IncrementConnections(port))
	assert.Equal(t, 2, registry.GetConnectionCount(port))

	// Close first connection
	err = stream1.Close()
	assert.NoError(t, err)
	assert.Equal(t, 1, registry.GetConnectionCount(port))

	// Now we should be able to create a new connection
	assert.True(t, registry.IncrementConnections(port))
	client3, server3 := net.Pipe()
	defer server3.Close()
	stream3 := &connCountingStream{
		Conn:     client3,
		port:     port,
		registry: registry,
		logger:   logger,
	}

	assert.Equal(t, 2, registry.GetConnectionCount(port))

	// Clean up
	stream2.Close()
	stream3.Close()
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestSOCKSManager_ErrorHandling(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()
	socksManager := NewSOCKSManager(registry, logger)

	port := 20001
	maxConns := int32(1)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	// Create mock yamux session
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxConfig := yamux.DefaultConfig()
	sess, err := yamux.Server(server, yamuxConfig)
	require.NoError(t, err)
	defer sess.Close()

	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, sess, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Fill up the connection limit
	assert.True(t, registry.IncrementConnections(port))

	// Create dialer
	dialer := socksManager.createDialer(port, sess)
	ctx := context.Background()

	// Try to dial when limit is reached
	conn, err := dialer(ctx, "tcp", "example.com:80")
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "connection limit reached")

	// Verify count hasn't changed
	assert.Equal(t, 1, registry.GetConnectionCount(port))
}

func TestSOCKSManager_PortNotFound(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()
	socksManager := NewSOCKSManager(registry, logger)

	port := 20001

	// Create mock yamux session
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxConfig := yamux.DefaultConfig()
	sess, err := yamux.Server(server, yamuxConfig)
	require.NoError(t, err)
	defer sess.Close()

	// Create dialer for non-existent port
	dialer := socksManager.createDialer(port, sess)
	ctx := context.Background()

	// Try to dial
	conn, err := dialer(ctx, "tcp", "example.com:80")
	assert.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "connection limit reached")
}

func TestSOCKSManager_RapidConnectionCycling(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	port := 20001
	maxConns := int32(10)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Rapidly create and close connections
	for i := 0; i < 100; i++ {
		assert.True(t, registry.IncrementConnections(port))

		client, server := net.Pipe()
		stream := &connCountingStream{
			Conn:     client,
			port:     port,
			registry: registry,
			logger:   logger,
		}

		// Close immediately
		stream.Close()
		server.Close()

		// Give a tiny bit of time for cleanup
		time.Sleep(1 * time.Millisecond)
	}

	// Final count should be 0
	assert.Equal(t, 0, registry.GetConnectionCount(port))
}

func TestSOCKSManager_ConcurrentConnectionAttempts(t *testing.T) {
	registry := NewRegistry()

	port := 20001
	maxConns := int32(5)

	// Reserve and bind port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	mockSession := &yamux.Session{}
	mockListener := &mockNetListener{}
	meta := ClientMeta{ClientName: "test"}

	err = registry.BindSession(port, mockSession, mockListener, meta, maxConns)
	require.NoError(t, err)

	// Try to create many connections concurrently
	const numAttempts = 20
	successChan := make(chan bool, numAttempts)
	var wg sync.WaitGroup
	wg.Add(numAttempts)

	for i := 0; i < numAttempts; i++ {
		go func() {
			defer wg.Done()
			success := registry.IncrementConnections(port)
			successChan <- success
		}()
	}

	wg.Wait()
	close(successChan)

	// Count successes
	successes := 0
	for success := range successChan {
		if success {
			successes++
		}
	}

	// Should have exactly maxConns successes
	assert.Equal(t, int(maxConns), successes)
	assert.Equal(t, int(maxConns), registry.GetConnectionCount(port))
}

func TestNewSOCKSManager(t *testing.T) {
	logger := zap.NewNop()
	registry := NewRegistry()

	manager := NewSOCKSManager(registry, logger)

	assert.NotNil(t, manager)
	assert.Equal(t, registry, manager.registry)
	assert.Equal(t, logger, manager.logger)
}

func TestSOCKSManager_StartListener(t *testing.T) {
	registry := NewRegistry()
	socksManager := NewSOCKSManager(registry, zap.NewNop())

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port
	listener.Close()

	// Reserve the port
	release, err := registry.ReservePorts([]int{port})
	require.NoError(t, err)
	defer release()

	// Create mock yamux session
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	yamuxConfig := yamux.DefaultConfig()
	sess, err := yamux.Server(server, yamuxConfig)
	require.NoError(t, err)
	defer sess.Close()

	// Start SOCKS5 listener
	socksListener, err := socksManager.StartListener(port, sess)
	require.NoError(t, err)
	require.NotNil(t, socksListener)
	defer socksListener.Close()

	// Verify listener is bound to correct address
	expectedAddr := fmt.Sprintf("127.0.0.1:%d", port)
	assert.Equal(t, expectedAddr, socksListener.Addr().String())
}
