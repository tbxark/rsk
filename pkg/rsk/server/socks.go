package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
)

type SOCKSManager struct {
	registry *Registry    // Port registry
	logger   *slog.Logger // Logger instance
}

// connCountingStream wraps a net.Conn to decrement connection count on close
type connCountingStream struct {
	net.Conn
	port      int
	registry  *Registry
	logger    *slog.Logger
	closeOnce sync.Once
}

func (c *connCountingStream) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.Conn.Close()
		c.registry.DecrementConnections(c.port)
		c.logger.Debug("Connection closed, decremented count",
			"port", c.port,
			"remaining", c.registry.GetConnectionCount(c.port))
	})
	return err
}

// NewSOCKSManager creates a new SOCKSManager instance
func NewSOCKSManager(registry *Registry, logger *slog.Logger) *SOCKSManager {
	return &SOCKSManager{
		registry: registry,
		logger:   logger,
	}
}

func (m *SOCKSManager) createDialer(port int, sess *yamux.Session) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Try to increment connection count before opening stream
		if !m.registry.IncrementConnections(port) {
			m.logger.Warn("Per-client connection limit reached",
				"port", port,
				"current", m.registry.GetConnectionCount(port))
			return nil, fmt.Errorf("connection limit reached for client")
		}

		// Ensure decrement happens when connection closes
		decremented := false
		defer func() {
			if !decremented {
				m.registry.DecrementConnections(port)
			}
		}()

		stream, err := sess.OpenStream()
		if err != nil {
			m.logger.Error("Failed to open yamux stream", "error", err)
			return nil, err
		}

		if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
			_ = stream.Close()
			return nil, err
		}

		if err := proto.WriteConnectReq(stream, addr); err != nil {
			_ = stream.Close()
			m.logger.Error("Failed to write CONNECT_REQ", "addr", addr, "error", err)
			return nil, err
		}

		if err := common.ClearDeadline(stream); err != nil {
			_ = stream.Close()
			return nil, err
		}

		// Wrap the stream to decrement on close
		decremented = true
		return &connCountingStream{
			Conn:     stream,
			port:     port,
			registry: m.registry,
			logger:   m.logger,
		}, nil
	}
}

// StartListener creates and starts a SOCKS5 server on the specified port.
func (m *SOCKSManager) StartListener(port int, sess *yamux.Session) (net.Listener, error) {
	conf := &socks5.Config{
		Dial: m.createDialer(port, sess),
	}

	server, err := socks5.New(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind SOCKS5 listener on %s: %w", addr, err)
	}

	go func() {
		m.logger.Info("SOCKS5 listener started", "port", port)
		if err := server.Serve(listener); err != nil {
			if opErr, ok := err.(*net.OpError); !ok || opErr.Err.Error() != "use of closed network connection" {
				m.logger.Error("SOCKS5 server error", "port", port, "error", err)
			}
		}
		m.logger.Info("SOCKS5 listener stopped", "port", port)
	}()

	return listener, nil
}
