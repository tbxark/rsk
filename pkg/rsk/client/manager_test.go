package client

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	// Test with nil logger
	m1 := NewManager(nil)
	if m1 == nil {
		t.Fatal("NewManager returned nil")
	}
	if m1.logger == nil {
		t.Fatal("Manager logger should not be nil")
	}

	// Test with provided logger
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m2 := NewManager(logger)
	if m2 == nil {
		t.Fatal("NewManager returned nil")
	}
	if m2.logger != logger {
		t.Fatal("Manager logger should match provided logger")
	}
}

func TestManagerOptions(t *testing.T) {
	cfg := &Config{
		ServerAddr:           "localhost:8080",
		Token:                []byte("test-token-16-bytes-minimum"),
		Port:                 20001,
		Name:                 "test-client",
		DialTimeout:          10 * time.Second,
		AllowPrivateNetworks: false,
		BlockedNetworks:      nil,
	}

	opts := ManagerOptions{
		Config:            cfg,
		AutoRestart:       true,
		RestartDelay:      3 * time.Second,
		MaxRestartRetries: 5,
	}

	if opts.Config.ServerAddr != "localhost:8080" {
		t.Errorf("Expected ServerAddr localhost:8080, got %s", opts.Config.ServerAddr)
	}
	if opts.Config.Port != 20001 {
		t.Errorf("Expected Port 20001, got %d", opts.Config.Port)
	}
	if !opts.AutoRestart {
		t.Error("Expected AutoRestart to be true")
	}
	if opts.RestartDelay != 3*time.Second {
		t.Errorf("Expected RestartDelay 3s, got %v", opts.RestartDelay)
	}
	if opts.MaxRestartRetries != 5 {
		t.Errorf("Expected MaxRestartRetries 5, got %d", opts.MaxRestartRetries)
	}
}

func TestManagerStatus(t *testing.T) {
	manager := NewManager(nil)

	// Initial status
	status := manager.GetStatus()
	if status.Running {
		t.Error("Manager should not be running initially")
	}
	if status.Port != 0 {
		t.Errorf("Expected port 0, got %d", status.Port)
	}
	if status.RestartCount != 0 {
		t.Errorf("Expected restart count 0, got %d", status.RestartCount)
	}
}

func TestManagerGetters(t *testing.T) {
	manager := NewManager(nil)

	// Test IsRunning
	if manager.IsRunning() {
		t.Error("Manager should not be running initially")
	}

	// Test GetPort
	if port := manager.GetPort(); port != 0 {
		t.Errorf("Expected port 0, got %d", port)
	}

	// Test GetUptime
	if uptime := manager.GetUptime(); uptime != 0 {
		t.Errorf("Expected uptime 0, got %v", uptime)
	}

	// Test GetRestartCount
	if count := manager.GetRestartCount(); count != 0 {
		t.Errorf("Expected restart count 0, got %d", count)
	}

	// Test GetLastError
	if err := manager.GetLastError(); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestManagerValidation(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	tests := []struct {
		name    string
		opts    ManagerOptions
		wantErr bool
	}{
		{
			name: "missing config",
			opts: ManagerOptions{
				Config: nil,
			},
			wantErr: true,
		},
		{
			name: "missing server address",
			opts: ManagerOptions{
				Config: &Config{
					Token:       []byte("test-token-16-bytes-minimum"),
					Port:        20001,
					Name:        "test",
					DialTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "missing token",
			opts: ManagerOptions{
				Config: &Config{
					ServerAddr:  "localhost:8080",
					Port:        20001,
					Name:        "test",
					DialTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "token too short",
			opts: ManagerOptions{
				Config: &Config{
					ServerAddr:  "localhost:8080",
					Token:       []byte("short"),
					Port:        20001,
					Name:        "test",
					DialTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid port (too low)",
			opts: ManagerOptions{
				Config: &Config{
					ServerAddr:  "localhost:8080",
					Token:       []byte("test-token-16-bytes-minimum"),
					Port:        -1,
					Name:        "test",
					DialTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid port (too high)",
			opts: ManagerOptions{
				Config: &Config{
					ServerAddr:  "localhost:8080",
					Token:       []byte("test-token-16-bytes-minimum"),
					Port:        70000,
					Name:        "test",
					DialTimeout: 10 * time.Second,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.Start(ctx, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestManagerStop(t *testing.T) {
	manager := NewManager(nil)

	// Stopping when not running should not panic
	manager.Stop()

	// Should be safe to call multiple times
	manager.Stop()
	manager.Stop()
}

func TestManagerStopAndWait(t *testing.T) {
	manager := NewManager(nil)

	// StopAndWait when not running should return quickly
	err := manager.StopAndWait(1 * time.Second)
	if err != nil {
		t.Errorf("StopAndWait() error = %v, want nil", err)
	}
}

func TestManagerContextCancellation(t *testing.T) {
	manager := NewManager(nil)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	opts := ManagerOptions{
		Config: &Config{
			ServerAddr:           "localhost:8080",
			Token:                []byte("test-token-16-bytes-minimum"),
			Port:                 20001,
			Name:                 "test-client",
			DialTimeout:          10 * time.Second,
			AllowPrivateNetworks: false,
		},
	}

	// Start will fail because server is not running, but that's ok for this test
	_, _ = manager.Start(ctx, opts)

	// Cancel the context
	cancel()

	// Give it a moment to process cancellation
	time.Sleep(200 * time.Millisecond)

	// Manager should stop
	if manager.IsRunning() {
		t.Error("Manager should have stopped after context cancellation")
	}
}

func TestManagerWithTimeout(t *testing.T) {
	manager := NewManager(nil)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	opts := ManagerOptions{
		Config: &Config{
			ServerAddr:           "localhost:8080",
			Token:                []byte("test-token-16-bytes-minimum"),
			Port:                 20001,
			Name:                 "test-client",
			DialTimeout:          10 * time.Second,
			AllowPrivateNetworks: false,
		},
	}

	// Start will fail because server is not running
	_, _ = manager.Start(ctx, opts)

	// Wait for timeout
	time.Sleep(200 * time.Millisecond)

	// Manager should have stopped due to context timeout
	if manager.IsRunning() {
		t.Error("Manager should have stopped after context timeout")
	}
}
