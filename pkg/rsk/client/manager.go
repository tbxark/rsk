package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// ManagerOptions contains configuration for starting the RSK client manager.
type ManagerOptions struct {
	Config *Config // Client configuration

	// Optional: Auto-restart configuration
	AutoRestart       bool          // Enable automatic restart on failure
	RestartDelay      time.Duration // Delay between restart attempts (default: 5s)
	MaxRestartRetries int           // Maximum restart attempts (0 = unlimited)
}

// ManagerStatus represents the current state of the RSK client manager.
type ManagerStatus struct {
	Running      bool      // Whether the client is running
	Port         int       // The port being used
	Message      string    // Status message
	StartTime    time.Time // When the client was started
	RestartCount int       // Number of times restarted
	LastError    error     // Last error encountered
	AutoRestart  bool      // Whether auto-restart is enabled
	ShuttingDown bool      // Whether graceful shutdown is in progress
}

// Manager manages a single RSK client instance with auto-restart and graceful shutdown support.
// It provides a higher-level API for managing client lifecycle, including automatic restart
// on failure and graceful shutdown capabilities.
type Manager struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	running      bool
	port         int
	status       string
	logger       *slog.Logger
	startTime    time.Time
	restartCount int
	lastError    error
	shuttingDown bool

	// Auto-restart configuration
	autoRestart       bool
	restartDelay      time.Duration
	maxRestartRetries int
	restartCtx        context.Context
	restartCancel     context.CancelFunc
}

// NewManager creates a new Manager instance.
// If logger is nil, a no-op logger will be used.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Manager{
		logger: logger,
	}
}

// Start starts the RSK client with the given options.
// The provided context controls the client lifecycle. When the context is canceled,
// the client will shut down gracefully.
// Returns the port being used, or an error if startup fails.
// If the client is already running, returns the current port.
func (m *Manager) Start(ctx context.Context, opts ManagerOptions) (int, error) {
	// Validate config
	if opts.Config == nil {
		return 0, errors.New("config is required")
	}

	clientCfg := opts.Config

	// Validate configuration
	if err := clientCfg.Validate(); err != nil {
		m.setStatus(false, 0, fmt.Sprintf("Configuration error: %v", err))
		return 0, fmt.Errorf("invalid configuration: %w", err)
	}

	m.mu.Lock()
	// Check if already running
	if m.running {
		port := m.port
		m.mu.Unlock()
		m.logger.Info("Client already running", "port", port)
		return port, nil
	}
	m.status = "Preparing to connect..."
	m.shuttingDown = false
	m.mu.Unlock()

	// Get port from config
	port := clientCfg.Port

	// Set auto-restart configuration
	m.mu.Lock()
	m.autoRestart = opts.AutoRestart
	m.restartDelay = opts.RestartDelay
	if m.restartDelay == 0 {
		m.restartDelay = 5 * time.Second
	}
	m.maxRestartRetries = opts.MaxRestartRetries
	m.restartCount = 0
	m.lastError = nil
	m.mu.Unlock()

	// Create context for client lifecycle (child of provided context)
	clientCtx, clientCancel := context.WithCancel(ctx)

	// Create context for auto-restart loop
	restartCtx, restartCancel := context.WithCancel(context.Background())

	m.mu.Lock()
	m.ctx = clientCtx
	m.cancel = clientCancel
	m.restartCtx = restartCtx
	m.restartCancel = restartCancel
	m.running = true
	m.port = port
	m.startTime = time.Now()
	m.status = fmt.Sprintf("Started on port %d", port)
	m.mu.Unlock()

	m.logger.Info("Starting RSK client",
		"server", clientCfg.ServerAddr,
		"port", port,
		"name", clientCfg.Name,
		"auto_restart", opts.AutoRestart)

	// Monitor parent context cancellation
	go func() {
		<-ctx.Done()
		m.logger.Info("Parent context canceled, stopping client")
		m.Stop()
	}()

	// Start client with optional auto-restart
	if opts.AutoRestart {
		go m.runWithAutoRestart(clientCfg)
	} else {
		go m.runOnce(clientCfg)
	}

	return port, nil
}

// runOnce runs the client once without auto-restart
func (m *Manager) runOnce(cfg *Config) {
	rskClient := &Client{
		Config:         cfg,
		ReconnectDelay: 2 * time.Second,
		Logger:         m.logger,
	}

	err := rskClient.Run(m.ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		m.logger.Error("Client stopped with error", "error", err)
		m.mu.Lock()
		m.lastError = err
		m.mu.Unlock()
		m.setStatus(false, m.port, fmt.Sprintf("Error: %v", err))
		return
	}

	m.logger.Info("Client stopped normally")
	m.setStatus(false, m.port, "Stopped")
}

// runWithAutoRestart runs the client with automatic restart on failure
func (m *Manager) runWithAutoRestart(cfg *Config) {
	attempt := 0

	for {
		// Check if we should stop
		select {
		case <-m.restartCtx.Done():
			m.logger.Info("Auto-restart loop stopped")
			return
		default:
		}

		// Check restart limit
		m.mu.Lock()
		maxRetries := m.maxRestartRetries
		shuttingDown := m.shuttingDown
		m.mu.Unlock()

		if shuttingDown {
			return
		}

		if maxRetries > 0 && attempt >= maxRetries {
			m.logger.Warn("Max restart retries reached", "max_retries", maxRetries)
			m.setStatus(false, m.port, fmt.Sprintf("Max restart retries (%d) reached", maxRetries))
			return
		}

		// Create new context for this attempt
		ctx, cancel := context.WithCancel(m.restartCtx)
		m.mu.Lock()
		m.ctx = ctx
		m.cancel = cancel
		m.mu.Unlock()

		if attempt > 0 {
			m.logger.Info("Restarting client", "attempt", attempt+1)
			m.mu.Lock()
			m.restartCount++
			m.mu.Unlock()
		}

		// Run client
		rskClient := &Client{
			Config:         cfg,
			ReconnectDelay: 2 * time.Second,
			Logger:         m.logger,
		}

		err := rskClient.Run(ctx)

		// Check if it was a graceful shutdown
		if errors.Is(err, context.Canceled) {
			m.logger.Info("Client stopped gracefully")
			m.setStatus(false, m.port, "Stopped")
			return
		}

		// Handle error
		if err != nil {
			m.logger.Error("Client stopped with error", "error", err)
			m.mu.Lock()
			m.lastError = err
			m.mu.Unlock()
			m.setStatus(false, m.port, fmt.Sprintf("Error: %v", err))
		}

		var hsErr *HandshakeError
		if errors.As(err, &hsErr) && hsErr.IsPortInUse() {
			m.logger.Warn("Port already in use, stopping auto-restart", "error", err)
			m.setStatus(false, m.port, fmt.Sprintf("Port in use: %v", err))
			return
		}

		attempt++

		// Wait before restart
		m.mu.Lock()
		delay := m.restartDelay
		m.mu.Unlock()

		m.logger.Info("Waiting before restart", "delay", delay)

		select {
		case <-m.restartCtx.Done():
			return
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
}

// Stop stops the running RSK client gracefully.
// It stops the auto-restart loop and cancels the client context.
// This method returns immediately without waiting for shutdown to complete.
// Use StopAndWait if you need to wait for graceful shutdown.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		if m.logger != nil {
			m.logger.Info("Client not running, nothing to stop")
		}
		return
	}

	m.shuttingDown = true
	cancel := m.cancel
	restartCancel := m.restartCancel
	m.cancel = nil
	m.restartCancel = nil
	m.running = false
	m.status = "Stopping..."
	m.mu.Unlock()

	m.logger.Info("Initiating graceful shutdown")

	// Stop auto-restart loop first
	if restartCancel != nil {
		restartCancel()
	}

	// Then stop the client
	if cancel != nil {
		cancel()
	}
}

// StopAndWait stops the client and waits for it to fully shut down.
// Returns an error if shutdown times out.
func (m *Manager) StopAndWait(timeout time.Duration) error {
	m.Stop()

	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("shutdown timeout after %v", timeout)
		case <-ticker.C:
			m.mu.Lock()
			shuttingDown := m.shuttingDown
			running := m.running
			m.mu.Unlock()

			if !shuttingDown && !running {
				m.logger.Info("Graceful shutdown completed")
				return nil
			}
		}
	}
}

// GetStatus returns the current status of the RSK client.
func (m *Manager) GetStatus() ManagerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ManagerStatus{
		Running:      m.running,
		Port:         m.port,
		Message:      m.status,
		StartTime:    m.startTime,
		RestartCount: m.restartCount,
		LastError:    m.lastError,
		AutoRestart:  m.autoRestart,
		ShuttingDown: m.shuttingDown,
	}
}

// IsRunning returns true if the client is currently running.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// GetPort returns the port being used by the client.
// Returns 0 if the client is not running.
func (m *Manager) GetPort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.port
}

// GetUptime returns how long the client has been running.
// Returns 0 if the client is not running.
func (m *Manager) GetUptime() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return 0
	}
	return time.Since(m.startTime)
}

// GetRestartCount returns the number of times the client has been restarted.
func (m *Manager) GetRestartCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restartCount
}

// GetLastError returns the last error encountered by the client.
func (m *Manager) GetLastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastError
}

// setStatus updates the internal status (thread-safe).
func (m *Manager) setStatus(running bool, port int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = running
	m.port = port
	m.status = message
	if !running {
		m.shuttingDown = false
	}
}
