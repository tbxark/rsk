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
	ServerAddr     string        // Server address to connect to
	Token          []byte        // Authentication token
	Ports          []int         // Ports to claim on the server
	Name           string        // Client name for identification
	DialTimeout    time.Duration // Timeout for dialing target addresses
	ReconnectDelay time.Duration // Delay between reconnection attempts
	Logger         *zap.Logger   // Logger instance
}

func handleStream(stream net.Conn, dialTimeout time.Duration, logger *zap.Logger) {
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

	logger.Info("Connected to target", zap.String("addr", addr))

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
	conn, err := net.Dial("tcp", c.ServerAddr)
	if err != nil {
		return nil, err
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	ports := make([]uint16, len(c.Ports))
	for i, p := range c.Ports {
		ports[i] = uint16(p)
	}

	hello := proto.Hello{
		Magic:   [4]byte{'R', 'S', 'K', '1'},
		Version: proto.Version,
		Token:   c.Token,
		Ports:   ports,
		Name:    c.Name,
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
		zap.String("server", c.ServerAddr),
		zap.Ints("ports", c.Ports),
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

func (c *Client) handleStreams(session *yamux.Session) error {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return err
		}

		go handleStream(stream, c.DialTimeout, c.Logger)
	}
}

// Run starts the client with automatic reconnection.
func (c *Client) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			c.Logger.Info("Client shutting down")
			return ctx.Err()
		default:
		}

		c.Logger.Info("Connecting to server", zap.String("server", c.ServerAddr))

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
		err = c.handleStreams(session)

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
