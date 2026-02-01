package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/proto"
	"go.uber.org/zap"
)

func TestServerRateLimiterIntegration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
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
	cfg.ListenAddr = "127.0.0.1:17000"
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
	conn1, err := net.Dial("tcp", "127.0.0.1:17000")
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
	conn1.Close()

	// Test 2: Second failed auth attempt should trigger block
	conn2, err := net.Dial("tcp", "127.0.0.1:17000")
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
	conn2.Close()

	// Test 3: Third attempt should be blocked immediately
	conn3, err := net.Dial("tcp", "127.0.0.1:17000")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Connection should be closed immediately without response
	conn3.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn3.Read(buf)
	if err == nil {
		t.Error("Expected connection to be closed, but read succeeded")
	}
	conn3.Close()

	// Test 4: Wait for block to expire
	time.Sleep(150 * time.Millisecond)

	// Should be able to connect again (but will fail auth and start counting again)
	conn4, err := net.Dial("tcp", "127.0.0.1:17000")
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
	conn4.Close()

	// Test 5: After successful auth, counter should be reset
	// Try with wrong token again - should not be blocked immediately
	conn5, err := net.Dial("tcp", "127.0.0.1:17000")
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
	conn5.Close()

	// Cleanup
	cancel()
	time.Sleep(50 * time.Millisecond)
}
