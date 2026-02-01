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
	listenAddr string      // Address to listen for client connections
	bindIP     string      // IP address to bind SOCKS5 listeners
	token      []byte      // Authentication token
	portMin    int         // Minimum allowed port
	portMax    int         // Maximum allowed port
	registry   *Registry   // Port registry
	logger     *zap.Logger // Logger instance
}

// handleClientConnection handles a single client connection through the complete lifecycle:
// handshake, validation, port reservation, session establishment, and cleanup.
func handleClientConnection(
	conn net.Conn,
	token []byte,
	portMin, portMax int,
	registry *Registry,
	socksManager *SOCKSManager,
	logger *zap.Logger,
) {
	defer func() {
		_ = conn.Close()
	}()

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
		sendErrorResponse(conn, proto.StatusAuthFail, "Authentication failed", logger)
		return
	}

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

		if err := registry.BindSession(port, session, socksListener, clientMeta); err != nil {
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

// NewServer creates a new Server.
func NewServer(listenAddr, bindIP string, token []byte, portMin, portMax int, logger *zap.Logger) *Server {
	return &Server{
		listenAddr: listenAddr,
		bindIP:     bindIP,
		token:      token,
		portMin:    portMin,
		portMax:    portMax,
		registry:   NewRegistry(),
		logger:     logger,
	}
}

// Start starts the server and accepts client connections.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}
	defer func() {
		_ = listener.Close()
	}()

	s.logger.Info("Server listening", zap.String("address", s.listenAddr))

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

		s.logger.Info("Accepted new connection", zap.String("remote_addr", conn.RemoteAddr().String()))

		go handleClientConnection(
			conn,
			s.token,
			s.portMin,
			s.portMax,
			s.registry,
			socksManager,
			s.logger,
		)
	}
}
