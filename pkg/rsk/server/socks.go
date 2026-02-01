package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
	"go.uber.org/zap"
)

type SOCKSManager struct {
	registry *Registry   // Port registry
	logger   *zap.Logger // Logger instance
}

// NewSOCKSManager creates a new SOCKSManager instance
func NewSOCKSManager(registry *Registry, logger *zap.Logger) *SOCKSManager {
	return &SOCKSManager{
		registry: registry,
		logger:   logger,
	}
}

func (m *SOCKSManager) createDialer(sess *yamux.Session) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		stream, err := sess.OpenStream()
		if err != nil {
			m.logger.Error("Failed to open yamux stream", zap.Error(err))
			return nil, err
		}

		if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
			_ = stream.Close()
			return nil, err
		}

		if err := proto.WriteConnectReq(stream, addr); err != nil {
			_ = stream.Close()
			m.logger.Error("Failed to write CONNECT_REQ", zap.String("addr", addr), zap.Error(err))
			return nil, err
		}

		if err := common.ClearDeadline(stream); err != nil {
			_ = stream.Close()
			return nil, err
		}

		return stream, nil
	}
}

// StartListener creates and starts a SOCKS5 server on the specified port.
func (m *SOCKSManager) StartListener(port int, sess *yamux.Session) (net.Listener, error) {
	conf := &socks5.Config{
		Dial: m.createDialer(sess),
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
		m.logger.Info("SOCKS5 listener started", zap.Int("port", port))
		if err := server.Serve(listener); err != nil {
			if opErr, ok := err.(*net.OpError); !ok || opErr.Err.Error() != "use of closed network connection" {
				m.logger.Error("SOCKS5 server error", zap.Int("port", port), zap.Error(err))
			}
		}
		m.logger.Info("SOCKS5 listener stopped", zap.Int("port", port))
	}()

	return listener, nil
}
