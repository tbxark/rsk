package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/proto"
	"go.uber.org/zap"
)

type Server struct {
	config   *Config     // Server configuration
	registry *Registry   // Port registry
	logger   *zap.Logger // Logger instance
}

// handleClientConnection handles a single client connection through the complete lifecycle:
// handshake, validation, port reservation, session establishment, and cleanup.
func handleClientConnection(
	conn net.Conn,
	connLimiter *ConnectionLimiter,
	rateLimiter *IPRateLimiter,
	token []byte,
	portMin, portMax int,
	maxConnsPerClient int,
	registry *Registry,
	socksManager *SOCKSManager,
	logger *zap.Logger,
) {
	defer connLimiter.Release()
	defer func() {
		_ = conn.Close()
	}()

	// Extract remote IP from connection
	remoteIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		logger.Error("Failed to extract remote IP", zap.Error(err))
		return
	}

	// Check if IP is blocked due to rate limiting
	if rateLimiter.IsBlocked(remoteIP) {
		logger.Warn("Connection blocked due to rate limiting",
			zap.String("remote_ip", remoteIP))
		return
	}

	if err := common.SetReadDeadline(conn, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline", zap.Error(err))
		return
	}

	// Read and parse HELLO message
	hello, err := proto.ReadHello(conn)
	if err != nil {
		logger.Warn("Failed to read HELLO message", zap.Error(err))
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid HELLO message", logger)
		return
	}

	logger.Info("Received HELLO message",
		zap.String("name", hello.Name),
		zap.Int("port_count", len(hello.Ports)),
		zap.Uint16s("ports", hello.Ports))

	if string(hello.Magic[:]) != proto.MagicValue {
		logger.Warn("Invalid MAGIC field")
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid MAGIC field", logger)
		return
	}

	if hello.Version != proto.Version {
		logger.Warn("Invalid VERSION field", zap.Uint8("version", hello.Version))
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid VERSION field", logger)
		return
	}

	if !common.TokenEqual(hello.Token, token) {
		logger.Warn("Token mismatch - authentication failed")

		// Record authentication failure and check if should block
		shouldBlock := rateLimiter.RecordFailure(remoteIP)
		if shouldBlock {
			logger.Warn("IP blocked due to authentication failures",
				zap.String("remote_ip", remoteIP))
		}

		sendErrorResponse(conn, proto.StatusAuthFail, "Authentication failed", logger)
		return
	}

	// Reset rate limiter on successful authentication
	rateLimiter.Reset(remoteIP)

	for _, port := range hello.Ports {
		if int(port) < portMin || int(port) > portMax {
			logger.Warn("Port outside allowed range",
				zap.Uint16("port", port),
				zap.Int("min", portMin),
				zap.Int("max", portMax))
			sendErrorResponse(conn, proto.StatusPortForbidden,
				fmt.Sprintf("Port %d outside allowed range %d-%d", port, portMin, portMax), logger)
			return
		}
	}

	logger.Info("HELLO validation successful", zap.String("client", hello.Name))

	ports := make([]int, len(hello.Ports))
	for i, p := range hello.Ports {
		ports[i] = int(p)
	}

	releaseFunc, err := registry.ReservePorts(ports)
	if err != nil {
		logger.Warn("Port reservation failed", zap.Error(err))
		sendErrorResponse(conn, proto.StatusPortInUse, "One or more ports are already in use", logger)
		return
	}

	portsReserved := true
	defer func() {
		if portsReserved {
			releaseFunc()
		}
	}()

	listeners := make(map[int]net.Listener)
	for _, port := range ports {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Warn("Failed to bind port", zap.Int("port", port), zap.Error(err))
			for _, l := range listeners {
				_ = l.Close()
			}
			sendErrorResponse(conn, proto.StatusPortInUse,
				fmt.Sprintf("Failed to bind port %d", port), logger)
			return
		}
		listeners[port] = listener
	}

	logger.Info("Ports bound successfully", zap.Ints("ports", ports))

	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        proto.StatusOK,
		AcceptedPorts: hello.Ports,
		Message:       "Connection accepted",
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", zap.Error(err))
		for _, l := range listeners {
			_ = l.Close()
		}
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write HELLO_RESP", zap.Error(err))
		for _, l := range listeners {
			_ = l.Close()
		}
		return
	}

	logger.Info("Sent HELLO_RESP with OK status")

	if err := common.ClearDeadline(conn); err != nil {
		logger.Error("Failed to clear deadline", zap.Error(err))
		for _, l := range listeners {
			_ = l.Close()
		}
		return
	}

	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.EnableKeepAlive = true
	yamuxConfig.KeepAliveInterval = 30 * time.Second
	yamuxConfig.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		logger.Error("Failed to create yamux session", zap.Error(err))
		for _, l := range listeners {
			_ = l.Close()
		}
		return
	}

	logger.Info("Yamux session created")

	clientID := uuid.New().String()
	clientMeta := ClientMeta{
		ClientName: hello.Name,
		ClientID:   clientID,
	}

	for _, port := range ports {
		if tcpListener, ok := listeners[port]; ok {
			_ = tcpListener.Close()
		}

		socksListener, err := socksManager.StartListener(port, session)
		if err != nil {
			logger.Error("Failed to start SOCKS5 listener", zap.Int("port", port), zap.Error(err))
			_ = session.Close()
			for _, l := range listeners {
				_ = l.Close()
			}
			return
		}

		if err := registry.BindSession(port, session, socksListener, clientMeta, int32(maxConnsPerClient)); err != nil {
			logger.Error("Failed to bind session to port", zap.Int("port", port), zap.Error(err))
			_ = session.Close()
			_ = socksListener.Close()
			return
		}
	}

	portsReserved = false

	logger.Info("Client session established",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name),
		zap.Ints("ports", ports))

	<-session.CloseChan()

	logger.Info("Session closed, starting cleanup",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name))

	registry.ReleasePorts(ports)

	logger.Info("Cleanup completed",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name),
		zap.Ints("ports", ports))
}

func sendErrorResponse(conn net.Conn, status uint8, message string, logger *zap.Logger) {
	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        status,
		AcceptedPorts: nil,
		Message:       message,
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", zap.Error(err))
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write error response", zap.Uint8("status", status), zap.Error(err))
	}
}

// NewServer creates a new Server from the provided configuration.
func NewServer(config *Config, logger *zap.Logger) *Server {
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

	s.logger.Info("Server listening", zap.String("address", s.config.ListenAddr))

	// Create connection limiter
	connLimiter := NewConnectionLimiter(s.config.MaxClients)
	s.logger.Info("Connection limiter initialized", zap.Int("max_clients", s.config.MaxClients))

	// Create rate limiter
	rateLimiter := NewRateLimiter(s.config.MaxAuthFailures, s.config.AuthBlockDuration)
	defer rateLimiter.Close()
	s.logger.Info("Rate limiter initialized",
		zap.Int("max_auth_failures", s.config.MaxAuthFailures),
		zap.Duration("auth_block_duration", s.config.AuthBlockDuration))

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
				s.logger.Error("Failed to accept connection", zap.Error(err))
				continue
			}
		}

		s.logger.Debug("Accepted new connection", zap.String("remote_addr", conn.RemoteAddr().String()))

		// Try to acquire a connection slot
		if !connLimiter.Acquire() {
			s.logger.Warn("Connection limit reached, rejecting new connection",
				zap.String("remote_addr", conn.RemoteAddr().String()),
				zap.Int("max_clients", s.config.MaxClients),
				zap.Int("available", connLimiter.Available()))
			_ = conn.Close()
			continue
		}

		go handleClientConnection(
			conn,
			connLimiter,
			rateLimiter,
			s.config.Token,
			s.config.PortMin,
			s.config.PortMax,
			s.config.MaxConnsPerClient,
			s.registry,
			socksManager,
			s.logger,
		)
	}
}
