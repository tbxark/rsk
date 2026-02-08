package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/proto"
)

func getFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer func() { _ = l.Close() }()

	return l.Addr().(*net.TCPAddr).Port
}

func TestServerRateLimiterIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	token := []byte("test-token-12345")

	// Create server with low rate limit for testing
	cfg := &Config{
		ListenAddr:        "127.0.0.1:0", // Use port 0 for automatic assignment
		BindIP:            "127.0.0.1",
		Token:             token,
		PortMin:           20000,
		PortMax:           20010,
		MaxClients:        10,
		MaxAuthFailures:   2,                      // Max 2 failures
		AuthBlockDuration: 100 * time.Millisecond, // Short block duration for testing
		MaxConnsPerClient: 100,
	}
	srv := NewServer(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Get the actual listening address
	// Since we can't easily get it from the server, we'll use a fixed port for this test
	// Let's recreate with a fixed port
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Recreate with fixed port
	cfg.ListenAddr = "127.0.0.1:19527"
	srv = NewServer(cfg, logger)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// Test 1: First failed auth attempt should not block
	conn1, err := net.Dial("tcp", "127.0.0.1:19527")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	hello := proto.Hello{
		Magic:   [4]byte{'R', 'S', 'K', '1'},
		Version: proto.Version,
		Token:   []byte("wrong-token"),
		Name:    "test-client",
		Ports:   []uint16{20001},
	}

	if err := proto.WriteHello(conn1, hello); err != nil {
		t.Fatalf("Failed to write HELLO: %v", err)
	}

	// Should receive error response
	resp1, err := proto.ReadHelloResp(conn1)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp1.Status != proto.StatusAuthFail {
		t.Errorf("Expected StatusAuthFail, got %d", resp1.Status)
	}
	_ = conn1.Close()

	// Test 2: Second failed auth attempt should trigger block
	conn2, err := net.Dial("tcp", "127.0.0.1:19527")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if err := proto.WriteHello(conn2, hello); err != nil {
		t.Fatalf("Failed to write HELLO: %v", err)
	}

	resp2, err := proto.ReadHelloResp(conn2)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp2.Status != proto.StatusAuthFail {
		t.Errorf("Expected StatusAuthFail, got %d", resp2.Status)
	}
	_ = conn2.Close()

	// Test 3: Third attempt should be blocked immediately
	conn3, err := net.Dial("tcp", "127.0.0.1:19527")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Connection should be closed immediately without response
	_ = conn3.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn3.Read(buf)
	if err == nil {
		t.Error("Expected connection to be closed, but read succeeded")
	}
	_ = conn3.Close()

	// Test 4: Wait for block to expire
	time.Sleep(150 * time.Millisecond)

	// Should be able to connect again (but will fail auth and start counting again)
	conn4, err := net.Dial("tcp", "127.0.0.1:19527")
	if err != nil {
		t.Fatalf("Failed to connect after block expiration: %v", err)
	}

	// Use correct token this time to test successful auth resets counter
	hello.Token = token
	if err := proto.WriteHello(conn4, hello); err != nil {
		t.Fatalf("Failed to write HELLO: %v", err)
	}

	resp4, err := proto.ReadHelloResp(conn4)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp4.Status != proto.StatusOK {
		t.Errorf("Expected StatusOK after block expiration with correct token, got %d", resp4.Status)
	}
	_ = conn4.Close()

	// Test 5: After successful auth, counter should be reset
	// Try with wrong token again - should not be blocked immediately
	conn5, err := net.Dial("tcp", "127.0.0.1:19527")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	hello.Token = []byte("wrong-token-again")
	if err := proto.WriteHello(conn5, hello); err != nil {
		t.Fatalf("Failed to write HELLO: %v", err)
	}

	resp5, err := proto.ReadHelloResp(conn5)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp5.Status != proto.StatusAuthFail {
		t.Errorf("Expected StatusAuthFail, got %d", resp5.Status)
	}
	_ = conn5.Close()

	// Cleanup
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestHandleClientConnection_PartialListenerFailureCleansUp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	token := []byte("test-token-12345")
	registry := NewRegistry()
	connLimiter := NewConnectionLimiter(10)
	rateLimiter := NewRateLimiter(5, time.Second)
	defer rateLimiter.Close()
	socksManager := NewSOCKSManager(registry, logger, "127.0.0.1")
	if !connLimiter.Acquire() {
		t.Fatal("failed to acquire connection limiter slot for test setup")
	}

	port1 := getFreePort(t)
	occupiedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to occupy test port: %v", err)
	}
	defer func() { _ = occupiedListener.Close() }()
	port2 := occupiedListener.Addr().(*net.TCPAddr).Port

	serverListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create test server listener: %v", err)
	}
	defer func() { _ = serverListener.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := serverListener.Accept()
		if err != nil {
			return
		}
		handleClientConnection(
			conn,
			connLimiter,
			rateLimiter,
			token,
			1,
			65535,
			100,
			registry,
			socksManager,
			logger,
		)
	}()

	clientConn, err := net.Dial("tcp", serverListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to connect to test server: %v", err)
	}

	hello := proto.Hello{
		Magic:   [4]byte{'R', 'S', 'K', '1'},
		Version: proto.Version,
		Token:   token,
		Name:    "cleanup-test-client",
		Ports:   []uint16{uint16(port1), uint16(port2)},
	}

	if err := proto.WriteHello(clientConn, hello); err != nil {
		t.Fatalf("failed to write HELLO: %v", err)
	}

	resp, err := proto.ReadHelloResp(clientConn)
	if err != nil {
		t.Fatalf("failed to read HELLO_RESP: %v", err)
	}
	if resp.Status != proto.StatusOK {
		t.Fatalf("expected status OK, got %d", resp.Status)
	}
	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleClientConnection did not exit in time")
	}

	registry.mu.RLock()
	_, exists1 := registry.slots[port1]
	_, exists2 := registry.slots[port2]
	registry.mu.RUnlock()
	if exists1 || exists2 {
		t.Fatalf("expected both ports to be cleaned up, got slot states port1=%v port2=%v", exists1, exists2)
	}

	// Port1 was successfully started first; it must be released after rollback.
	rebind, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port1))
	if err != nil {
		t.Fatalf("expected port1 to be released after rollback: %v", err)
	}
	_ = rebind.Close()
}
