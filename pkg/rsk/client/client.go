package client

import (
	"context"
	"io"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
)

// Client connects to RSK server and handles outbound connections.
type Client struct {
	Config         *Config       // Client configuration
	ReconnectDelay time.Duration // Delay between reconnection attempts
	Logger         *zap.Logger   // Logger instance
}

func handleStream(stream net.Conn, dialTimeout time.Duration, filter *AddressFilter, logger *zap.Logger) {
	defer func() {
		_ = stream.Close()
	}()

	if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline for CONNECT_REQ", zap.Error(err))
		return
	}

	addr, err := proto.ReadConnectReq(stream)
	if err != nil {
		logger.Error("Failed to read CONNECT_REQ", zap.Error(err))
		return
	}

	logger.Debug("Received CONNECT_REQ", zap.String("addr", addr))

	// Validate address with filter
	if err := filter.IsAllowed(addr); err != nil {
		logger.Warn("Target address blocked by filter",
			zap.String("addr", addr),
			zap.Error(err))
		return
	}

	target, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		logger.Warn("Failed to dial target", zap.String("addr", addr), zap.Error(err))
		return
	}
	defer func() {
		_ = target.Close()
	}()

	if err := common.ClearDeadline(stream); err != nil {
		logger.Error("Failed to clear stream deadline", zap.Error(err))
		return
	}

	logger.Debug("Connected to target", zap.String("addr", addr))

	done := make(chan error, 2)

	go func() {
		_, err := io.Copy(target, stream)
		done <- err
	}()

	go func() {
		_, err := io.Copy(stream, target)
		done <- err
	}()

	err = <-done

	if err != nil && err != io.EOF {
		logger.Debug("Connection closed with error", zap.String("addr", addr), zap.Error(err))
	} else {
		logger.Debug("Connection closed", zap.String("addr", addr))
	}
}

func (c *Client) connect() (*yamux.Session, error) {
	conn, err := net.Dial("tcp", c.Config.ServerAddr)
	if err != nil {
		return nil, err
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	ports := make([]uint16, 1)
	ports[0] = uint16(c.Config.Port)

	hello := proto.Hello{
		Magic:   [4]byte{'R', 'S', 'K', '1'},
		Version: proto.Version,
		Token:   c.Config.Token,
		Ports:   ports,
		Name:    c.Config.Name,
	}

	if err := proto.WriteHello(conn, hello); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := common.SetReadDeadline(conn, 5*time.Second); err != nil {
		_ = conn.Close()
		return nil, err
	}

	resp, err := proto.ReadHelloResp(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := common.ClearDeadline(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if resp.Status != proto.StatusOK {
		_ = conn.Close()
		return nil, &HandshakeError{
			Status:  resp.Status,
			Message: resp.Message,
		}
	}

	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 30 * time.Second
	cfg.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Client(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	c.Logger.Info("Successfully connected to server",
		zap.String("server", c.Config.ServerAddr),
		zap.Int("port", c.Config.Port),
		zap.Uint16s("accepted_ports", resp.AcceptedPorts))

	return session, nil
}

// HandshakeError represents a HELLO handshake error.
type HandshakeError struct {
	Status  uint8  // Error status code
	Message string // Error message
}

func (e *HandshakeError) Error() string {
	statusName := "UNKNOWN"
	switch e.Status {
	case proto.StatusAuthFail:
		statusName = "AUTH_FAIL"
	case proto.StatusBadRequest:
		statusName = "BAD_REQUEST"
	case proto.StatusPortForbidden:
		statusName = "PORT_FORBIDDEN"
	case proto.StatusPortInUse:
		statusName = "PORT_IN_USE"
	case proto.StatusServerInternal:
		statusName = "SERVER_INTERNAL"
	}

	if e.Message != "" {
		return statusName + ": " + e.Message
	}
	return statusName
}

func (e *HandshakeError) IsAuthFail() bool {
	return e.Status == proto.StatusAuthFail
}

func (e *HandshakeError) IsPortInUse() bool {
	return e.Status == proto.StatusPortInUse
}

func (c *Client) handleStreams(session *yamux.Session, filter *AddressFilter) error {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return err
		}

		go handleStream(stream, c.Config.DialTimeout, filter, c.Logger)
	}
}

// Run starts the client with automatic reconnection.
func (c *Client) Run(ctx context.Context) error {
	// Create address filter
	filter, err := NewAddressFilter(c.Config.AllowPrivateNetworks, c.Config.BlockedNetworks)
	if err != nil {
		c.Logger.Error("Failed to create address filter", zap.Error(err))
		return err
	}

	c.Logger.Info("Address filter initialized",
		zap.Bool("allow_private", c.Config.AllowPrivateNetworks),
		zap.Int("blocked_networks_count", len(c.Config.BlockedNetworks)))

	for {
		select {
		case <-ctx.Done():
			c.Logger.Info("Client shutting down")
			return ctx.Err()
		default:
		}

		c.Logger.Info("Connecting to server", zap.String("server", c.Config.ServerAddr))

		session, err := c.connect()
		if err != nil {
			if hsErr, ok := err.(*HandshakeError); ok {
				if hsErr.IsAuthFail() {
					c.Logger.Error("Authentication failed, exiting", zap.Error(err))
					return err
				}

				if hsErr.IsPortInUse() {
					c.Logger.Error("Ports already in use, exiting", zap.Error(err))
					return err
				}

				c.Logger.Warn("Handshake failed, will retry",
					zap.Error(err),
					zap.Duration("delay", c.ReconnectDelay))
			} else {
				c.Logger.Warn("Connection failed, will retry",
					zap.Error(err),
					zap.Duration("delay", c.ReconnectDelay))
			}

			select {
			case <-time.After(c.ReconnectDelay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		c.Logger.Info("Session established, handling streams")
		err = c.handleStreams(session, filter)

		c.Logger.Warn("Session closed, will reconnect",
			zap.Error(err),
			zap.Duration("delay", c.ReconnectDelay))

		_ = session.Close()

		select {
		case <-time.After(c.ReconnectDelay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
