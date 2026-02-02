package client

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
)

// Client connects to RSK server and handles outbound connections.
type Client struct {
	Config         *Config
	ReconnectDelay time.Duration
	Logger         *slog.Logger
}

func handleStream(stream net.Conn, dialTimeout time.Duration, filter *AddressFilter, logger *slog.Logger) {
	defer func() {
		_ = stream.Close()
	}()

	if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline for CONNECT_REQ", "error", err)
		return
	}

	addr, err := proto.ReadConnectReq(stream)
	if err != nil {
		logger.Error("Failed to read CONNECT_REQ", "error", err)
		return
	}

	logger.Debug("Received CONNECT_REQ", "addr", addr)

	// Validate address with filter
	if err := filter.IsAllowed(addr); err != nil {
		logger.Warn("Target address blocked by filter",
			"addr", addr,
			"error", err)
		return
	}

	target, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		logger.Warn("Failed to dial target", "addr", addr, "error", err)
		return
	}
	defer func() {
		_ = target.Close()
	}()

	if err := common.ClearDeadline(stream); err != nil {
		logger.Error("Failed to clear stream deadline", "error", err)
		return
	}

	logger.Debug("Connected to target", "addr", addr)

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
		logger.Debug("Connection closed with error", "addr", addr, "error", err)
	} else {
		logger.Debug("Connection closed", "addr", addr)
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
		"server", c.Config.ServerAddr,
		"port", c.Config.Port,
		"accepted_ports", resp.AcceptedPorts)

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

// Run starts the client with automatic reconnection using exponential backoff.
func (c *Client) Run(ctx context.Context) error {
	// Create address filter
	filter, err := NewAddressFilter(c.Config.AllowPrivateNetworks, c.Config.BlockedNetworks)
	if err != nil {
		c.Logger.Error("Failed to create address filter", "error", err)
		return err
	}

	c.Logger.Info("Address filter initialized",
		"allow_private", c.Config.AllowPrivateNetworks,
		"blocked_networks_count", len(c.Config.BlockedNetworks))

	// Configure exponential backoff
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = c.ReconnectDelay
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 0 // Never stop retrying
	b.Multiplier = 2.0
	b.RandomizationFactor = 0.1

	backoffWithContext := backoff.WithContext(b, ctx)

	attempt := 0
	operation := func() error {
		attempt++

		select {
		case <-ctx.Done():
			c.Logger.Info("Client shutting down")
			return backoff.Permanent(ctx.Err())
		default:
		}

		c.Logger.Info("Connecting to server",
			"server", c.Config.ServerAddr,
			"attempt", attempt)

		session, err := c.connect()
		if err != nil {
			if hsErr, ok := err.(*HandshakeError); ok {
				if hsErr.IsAuthFail() {
					c.Logger.Error("Authentication failed, exiting", "error", err)
					return backoff.Permanent(err)
				}

				if hsErr.IsPortInUse() {
					c.Logger.Error("Ports already in use, exiting", "error", err)
					return backoff.Permanent(err)
				}

				c.Logger.Warn("Handshake failed, will retry", "error", err)
			} else {
				c.Logger.Warn("Connection failed, will retry", "error", err)
			}
			return err
		}

		// Reset attempt counter on successful connection
		attempt = 0
		b.Reset()

		c.Logger.Info("Session established, handling streams")
		err = c.handleStreams(session, filter)

		c.Logger.Warn("Session closed, will reconnect", "error", err)
		_ = session.Close()

		// Return error to trigger backoff
		return err
	}

	return backoff.Retry(operation, backoffWithContext)
}
