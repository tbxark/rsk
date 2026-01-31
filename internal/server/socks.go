package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/internal/common"
	"github.com/tbxark/rsk/internal/proto"
	"go.uber.org/zap"
)

// SOCKSManager manages SOCKS5 servers for client sessions
type SOCKSManager struct {
	registry *Registry
	logger   *zap.Logger
}

// NewSOCKSManager creates a new SOCKSManager instance
func NewSOCKSManager(registry *Registry, logger *zap.Logger) *SOCKSManager {
	return &SOCKSManager{
		registry: registry,
		logger:   logger,
	}
}

// createDialer creates a custom Dial function for SOCKS5 that routes through yamux streams.
// The returned function opens a yamux stream, writes a CONNECT_REQ, and returns the stream as net.Conn.
func (m *SOCKSManager) createDialer(sess *yamux.Session) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Open yamux stream
		stream, err := sess.OpenStream()
		if err != nil {
			m.logger.Error("Failed to open yamux stream", zap.Error(err))
			return nil, err
		}

		// Set temporary deadline for writing CONNECT_REQ
		if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
			stream.Close()
			return nil, err
		}

		// Write CONNECT_REQ with address
		if err := proto.WriteConnectReq(stream, addr); err != nil {
			stream.Close()
			m.logger.Error("Failed to write CONNECT_REQ", zap.String("addr", addr), zap.Error(err))
			return nil, err
		}

		// Clear deadline after successful write
		if err := common.ClearDeadline(stream); err != nil {
			stream.Close()
			return nil, err
		}

		// Return stream as net.Conn
		return stream, nil
	}
}

// StartListener creates and starts a SOCKS5 server on the specified port.
// The SOCKS5 server is bound to 127.0.0.1:port and routes connections through the yamux session.
// BIND and UDP ASSOCIATE commands are rejected.
// Returns the listener or an error if binding fails.
func (m *SOCKSManager) StartListener(port int, sess *yamux.Session) (net.Listener, error) {
	// Create SOCKS5 configuration with custom Dial
	// Note: go-socks5 by default rejects BIND and UDP ASSOCIATE commands
	conf := &socks5.Config{
		Dial: m.createDialer(sess),
	}

	// Create SOCKS5 server
	server, err := socks5.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	// Bind listener to 127.0.0.1:port
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind SOCKS5 listener on %s: %w", addr, err)
	}

	// Start serving in goroutine
	go func() {
		m.logger.Info("SOCKS5 listener started", zap.Int("port", port))
		if err := server.Serve(listener); err != nil {
			// Only log if it's not a normal close
			if opErr, ok := err.(*net.OpError); !ok || opErr.Err.Error() != "use of closed network connection" {
				m.logger.Error("SOCKS5 server error", zap.Int("port", port), zap.Error(err))
			}
		}
		m.logger.Info("SOCKS5 listener stopped", zap.Int("port", port))
	}()

	return listener, nil
}
