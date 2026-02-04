package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
)

type Server struct {
	config   *Config      // Server configuration
	registry *Registry    // Port registry
	logger   *slog.Logger // Logger instance
}

// handleClientConnection handles a single client connection through the complete lifecycle:
// handshake, validation, port reservation, session establishment, and cleanup.
func handleClientConnection(
	conn net.Conn,
	connLimiter *ConnectionLimiter,
	rateLimiter *IPRateLimiter,
	token []byte,
	bindIP string,
	portMin, portMax int,
	maxConnsPerClient int,
	registry *Registry,
	socksManager *SOCKSManager,
	logger *slog.Logger,
) {
	defer connLimiter.Release()
	defer func() {
		_ = conn.Close()
	}()

	// Extract remote IP from connection
	remoteIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		logger.Error("Failed to extract remote IP", "error", err)
		return
	}

	// Check if IP is blocked due to rate limiting
	if rateLimiter.IsBlocked(remoteIP) {
		logger.Warn("Connection blocked due to rate limiting",
			"remote_ip", remoteIP)
		return
	}

	if err := common.SetReadDeadline(conn, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline", "error", err)
		return
	}

	// Read and parse HELLO message
	hello, err := proto.ReadHello(conn)
	if err != nil {
		logger.Warn("Failed to read HELLO message", "error", err)
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid HELLO message", logger)
		return
	}

	logger.Info("Received HELLO message",
		"name", hello.Name,
		"port_count", len(hello.Ports),
		"ports", hello.Ports)

	if string(hello.Magic[:]) != proto.MagicValue {
		logger.Warn("Invalid MAGIC field")
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid MAGIC field", logger)
		return
	}

	if hello.Version != proto.Version {
		logger.Warn("Invalid VERSION field", "version", hello.Version)
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid VERSION field", logger)
		return
	}

	if !common.TokenEqual(hello.Token, token) {
		logger.Warn("Token mismatch - authentication failed")

		// Record authentication failure and check if should block
		shouldBlock := rateLimiter.RecordFailure(remoteIP)
		if shouldBlock {
			logger.Warn("IP blocked due to authentication failures",
				"remote_ip", remoteIP)
		}

		sendErrorResponse(conn, proto.StatusAuthFail, "Authentication failed", logger)
		return
	}

	// Reset rate limiter on successful authentication
	rateLimiter.Reset(remoteIP)

	for _, port := range hello.Ports {
		if int(port) < portMin || int(port) > portMax {
			logger.Warn("Port outside allowed range",
				"port", port,
				"min", portMin,
				"max", portMax)
			sendErrorResponse(conn, proto.StatusPortForbidden,
				fmt.Sprintf("Port %d outside allowed range %d-%d", port, portMin, portMax), logger)
			return
		}
	}

	logger.Info("HELLO validation successful", "client", hello.Name)

	ports := make([]int, len(hello.Ports))
	for i, p := range hello.Ports {
		ports[i] = int(p)
	}

	_, err = registry.ReservePorts(ports)
	if err != nil {
		logger.Warn("Port reservation failed", "error", err)
		sendErrorResponse(conn, proto.StatusPortInUse, "One or more ports are already in use", logger)
		return
	}

	var cleanupOnce sync.Once
	tcpListeners := make(map[int]net.Listener)
	socksListeners := make(map[int]net.Listener)
	cleanup := func() {
		cleanupOnce.Do(func() {
			for _, listener := range socksListeners {
				_ = listener.Close()
			}
			for _, listener := range tcpListeners {
				_ = listener.Close()
			}
			registry.ReleasePorts(ports)
		})
	}
	defer cleanup()

	for _, port := range ports {
		addr := fmt.Sprintf("%s:%d", bindIP, port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Warn("Failed to bind port", "port", port, "error", err)
			cleanup()
			sendErrorResponse(conn, proto.StatusPortInUse,
				fmt.Sprintf("Failed to bind port %d", port), logger)
			return
		}
		tcpListeners[port] = listener
	}

	logger.Info("Ports bound successfully", "ports", ports)

	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        proto.StatusOK,
		AcceptedPorts: hello.Ports,
		Message:       "Connection accepted",
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", "error", err)
		cleanup()
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write HELLO_RESP", "error", err)
		cleanup()
		return
	}

	logger.Info("Sent HELLO_RESP with OK status")

	if err := common.ClearDeadline(conn); err != nil {
		logger.Error("Failed to clear deadline", "error", err)
		cleanup()
		return
	}

	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.EnableKeepAlive = true
	yamuxConfig.KeepAliveInterval = 30 * time.Second
	yamuxConfig.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		logger.Error("Failed to create yamux session", "error", err)
		cleanup()
		return
	}

	logger.Info("Yamux session created")

	clientID := uuid.New().String()
	clientMeta := ClientMeta{
		ClientName: hello.Name,
		ClientID:   clientID,
	}

	for _, port := range ports {
		if tcpListener, ok := tcpListeners[port]; ok {
			_ = tcpListener.Close()
			delete(tcpListeners, port)
		}

		socksListener, err := socksManager.StartListener(port, bindIP, session)
		if err != nil {
			logger.Error("Failed to start SOCKS5 listener", "port", port, "error", err)
			_ = session.Close()
			cleanup()
			return
		}
		socksListeners[port] = socksListener

		if err := registry.BindSession(port, session, socksListener, clientMeta, int32(maxConnsPerClient)); err != nil {
			logger.Error("Failed to bind session to port", "port", port, "error", err)
			_ = session.Close()
			_ = socksListener.Close()
			delete(socksListeners, port)
			cleanup()
			return
		}
	}

	logger.Info("Client session established",
		"client_id", clientID,
		"client_name", hello.Name,
		"ports", ports)

	// Ensure cleanup happens even if session closes immediately
	<-session.CloseChan()
}

func sendErrorResponse(conn net.Conn, status uint8, message string, logger *slog.Logger) {
	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        status,
		AcceptedPorts: nil,
		Message:       message,
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", "error", err)
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write error response", "status", status, "error", err)
	}
}

// NewServer creates a new Server from the provided configuration.
func NewServer(config *Config, logger *slog.Logger) *Server {
	return &Server{
		config:   config,
		registry: NewRegistry(),
		logger:   logger,
	}
}

// Start starts the server and accepts client connections.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.config.ListenAddr, err)
	}
	defer func() {
		_ = listener.Close()
	}()

	s.logger.Info("Server listening", "address", s.config.ListenAddr)

	// Create connection limiter
	connLimiter := NewConnectionLimiter(s.config.MaxClients)
	s.logger.Info("Connection limiter initialized", "max_clients", s.config.MaxClients)

	// Create rate limiter
	rateLimiter := NewRateLimiter(s.config.MaxAuthFailures, s.config.AuthBlockDuration)
	defer rateLimiter.Close()
	s.logger.Info("Rate limiter initialized",
		"max_auth_failures", s.config.MaxAuthFailures,
		"auth_block_duration", s.config.AuthBlockDuration)

	socksManager := NewSOCKSManager(s.registry, s.logger)

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			s.logger.Info("Shutting down server")
			_ = listener.Close()
		case <-done:
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				s.logger.Error("Failed to accept connection", "error", err)
				continue
			}
		}

		s.logger.Debug("Accepted new connection", "remote_addr", conn.RemoteAddr().String())

		// Try to acquire a connection slot
		if !connLimiter.Acquire() {
			s.logger.Warn("Connection limit reached, rejecting new connection",
				"remote_addr", conn.RemoteAddr().String(),
				"max_clients", s.config.MaxClients,
				"available", connLimiter.Available())
			_ = conn.Close()
			continue
		}

		go handleClientConnection(
			conn,
			connLimiter,
			rateLimiter,
			s.config.Token,
			s.config.BindIP,
			s.config.PortMin,
			s.config.PortMax,
			s.config.MaxConnsPerClient,
			s.registry,
			socksManager,
			s.logger,
		)
	}
}
