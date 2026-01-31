package client

import (
	"context"
	"io"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/internal/common"
	"github.com/tbxark/rsk/internal/proto"
)

// Package client implements the RSK client components including
// the main client and stream handler.

// Client represents the RSK client that connects to the server and handles outbound connections
// Requirements: 13.5, 13.6, 13.7, 13.8, 13.9
type Client struct {
	ServerAddr     string        // Server address to connect to
	Token          []byte        // Authentication token
	Ports          []int         // Ports to claim on the server
	Name           string        // Client name for identification
	DialTimeout    time.Duration // Timeout for dialing target addresses
	ReconnectDelay time.Duration // Delay between reconnection attempts
	Logger         *zap.Logger   // Logger instance
}

// handleStream processes an incoming yamux stream by reading the CONNECT_REQ,
// dialing the target address, and forwarding data bidirectionally.
// Requirements: 9.3, 9.4, 10.1, 10.2, 10.3, 10.4, 10.5, 12.3
func handleStream(stream net.Conn, dialTimeout time.Duration, logger *zap.Logger) {
	defer stream.Close()

	// Set read deadline for CONNECT_REQ (Requirement 9.3, 10.1)
	if err := common.SetReadDeadline(stream, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline for CONNECT_REQ", zap.Error(err))
		return
	}

	// Read and parse CONNECT_REQ (Requirement 9.4, 10.1)
	addr, err := proto.ReadConnectReq(stream)
	if err != nil {
		logger.Error("Failed to read CONNECT_REQ", zap.Error(err))
		return
	}

	logger.Debug("Received CONNECT_REQ", zap.String("addr", addr))

	// Dial target address with timeout (Requirement 10.2)
	target, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		// If dial fails, close stream immediately (Requirement 10.4)
		logger.Warn("Failed to dial target", zap.String("addr", addr), zap.Error(err))
		return
	}
	defer target.Close()

	// Clear deadlines after successful dial (Requirement 12.3)
	if err := common.ClearDeadline(stream); err != nil {
		logger.Error("Failed to clear stream deadline", zap.Error(err))
		return
	}

	logger.Info("Connected to target", zap.String("addr", addr))

	// Bidirectional data forwarding (Requirement 10.3, 10.5)
	// Spawn two goroutines for io.Copy in each direction
	done := make(chan error, 2)

	// Stream -> Target
	go func() {
		_, err := io.Copy(target, stream)
		done <- err
	}()

	// Target -> Stream
	go func() {
		_, err := io.Copy(stream, target)
		done <- err
	}()

	// Wait for either copy to complete (Requirement 10.3)
	err = <-done

	// Close both connections (Requirement 10.5)
	// Deferred closes will handle cleanup
	if err != nil && err != io.EOF {
		logger.Debug("Connection closed with error", zap.String("addr", addr), zap.Error(err))
	} else {
		logger.Debug("Connection closed", zap.String("addr", addr))
	}
}

// connect establishes a connection to the server, performs the HELLO handshake,
// and creates a yamux client session on success.
// Requirements: 3.2, 4.1, 7.2, 12.2
func (c *Client) connect() (*yamux.Session, error) {
	// Dial server TCP connection (Requirement 3.2)
	conn, err := net.Dial("tcp", c.ServerAddr)
	if err != nil {
		return nil, err
	}

	// Set write deadline for HELLO (Requirement 12.2)
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}

	// Convert ports from []int to []uint16
	ports := make([]uint16, len(c.Ports))
	for i, p := range c.Ports {
		ports[i] = uint16(p)
	}

	// Write HELLO message (Requirement 4.1)
	hello := proto.Hello{
		Magic:   [4]byte{'R', 'S', 'K', '1'},
		Version: proto.Version,
		Token:   c.Token,
		Ports:   ports,
		Name:    c.Name,
	}

	if err := proto.WriteHello(conn, hello); err != nil {
		conn.Close()
		return nil, err
	}

	// Clear write deadline and set read deadline for HELLO_RESP
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}

	if err := common.SetReadDeadline(conn, 5*time.Second); err != nil {
		conn.Close()
		return nil, err
	}

	// Read HELLO_RESP (Requirement 7.2)
	resp, err := proto.ReadHelloResp(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Clear deadline after reading response
	if err := common.ClearDeadline(conn); err != nil {
		conn.Close()
		return nil, err
	}

	// Handle error statuses (Requirement 7.2)
	if resp.Status != proto.StatusOK {
		conn.Close()
		return nil, &HandshakeError{
			Status:  resp.Status,
			Message: resp.Message,
		}
	}

	// Create yamux.Client() session on success (Requirement 7.2)
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 30 * time.Second
	cfg.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Client(conn, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}

	c.Logger.Info("Successfully connected to server",
		zap.String("server", c.ServerAddr),
		zap.Ints("ports", c.Ports),
		zap.Uint16s("accepted_ports", resp.AcceptedPorts))

	return session, nil
}

// HandshakeError represents an error during the HELLO handshake
type HandshakeError struct {
	Status  uint8
	Message string
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

// IsAuthFail returns true if the error is an AUTH_FAIL status
func (e *HandshakeError) IsAuthFail() bool {
	return e.Status == proto.StatusAuthFail
}

// IsPortInUse returns true if the error is a PORT_IN_USE status
func (e *HandshakeError) IsPortInUse() bool {
	return e.Status == proto.StatusPortInUse
}

// handleStreams accepts incoming yamux streams and spawns goroutines to handle each one.
// Requirements: 9.3
func (c *Client) handleStreams(session *yamux.Session) error {
	for {
		// Accept stream (Requirement 9.3)
		stream, err := session.AcceptStream()
		if err != nil {
			// Session closed or error
			return err
		}

		// Spawn goroutine for each stream (Requirement 9.3)
		go handleStream(stream, c.DialTimeout, c.Logger)
	}
}

// Run starts the client with automatic reconnection logic.
// Requirements: 16.1, 16.2, 16.3, 16.4
func (c *Client) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			c.Logger.Info("Client shutting down")
			return ctx.Err()
		default:
		}

		c.Logger.Info("Connecting to server", zap.String("server", c.ServerAddr))

		// Call connect() (Requirement 16.1)
		session, err := c.connect()
		if err != nil {
			// Check if it's a handshake error
			if hsErr, ok := err.(*HandshakeError); ok {
				// On AUTH_FAIL: exit or long delay (Requirement 16.2, 16.3)
				if hsErr.IsAuthFail() {
					c.Logger.Error("Authentication failed, exiting", zap.Error(err))
					return err
				}

				// On PORT_IN_USE: exit (Requirement 16.4)
				if hsErr.IsPortInUse() {
					c.Logger.Error("Ports already in use, exiting", zap.Error(err))
					return err
				}

				// Other handshake errors: log and retry
				c.Logger.Warn("Handshake failed, will retry",
					zap.Error(err),
					zap.Duration("delay", c.ReconnectDelay))
			} else {
				// Connection error: log and retry (Requirement 16.1)
				c.Logger.Warn("Connection failed, will retry",
					zap.Error(err),
					zap.Duration("delay", c.ReconnectDelay))
			}

			// Wait reconnectDelay and retry (Requirement 16.1)
			select {
			case <-time.After(c.ReconnectDelay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Call handleStreams() after successful connection (Requirement 16.1)
		c.Logger.Info("Session established, handling streams")
		err = c.handleStreams(session)

		// Session closed or error
		c.Logger.Warn("Session closed, will reconnect",
			zap.Error(err),
			zap.Duration("delay", c.ReconnectDelay))

		// Close session
		session.Close()

		// Wait before reconnecting
		select {
		case <-time.After(c.ReconnectDelay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
