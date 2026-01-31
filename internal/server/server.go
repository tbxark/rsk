package server

import (
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/tbxark/rsk/internal/common"
	"github.com/tbxark/rsk/internal/proto"
	"go.uber.org/zap"
)

// Package server implements the RSK server components including
// the main server, registry, SOCKS5 manager, and connection handler.

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
	defer conn.Close()

	// Set 5-second read deadline for HELLO
	if err := common.SetReadDeadline(conn, 5*time.Second); err != nil {
		logger.Error("Failed to set read deadline", zap.Error(err))
		return
	}

	// Read and parse HELLO message
	hello, err := proto.ReadHello(conn)
	if err != nil {
		logger.Warn("Failed to read HELLO message", zap.Error(err))
		// Send BAD_REQUEST response
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid HELLO message", logger)
		return
	}

	logger.Info("Received HELLO message",
		zap.String("name", hello.Name),
		zap.Int("port_count", len(hello.Ports)),
		zap.Uint16s("ports", hello.Ports))

	// Validate MAGIC (already validated in ReadHello, but check for completeness)
	if string(hello.Magic[:]) != proto.MagicValue {
		logger.Warn("Invalid MAGIC field")
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid MAGIC field", logger)
		return
	}

	// Validate VERSION (already validated in ReadHello, but check for completeness)
	if hello.Version != proto.Version {
		logger.Warn("Invalid VERSION field", zap.Uint8("version", hello.Version))
		sendErrorResponse(conn, proto.StatusBadRequest, "Invalid VERSION field", logger)
		return
	}

	// Compare token using constant-time comparison
	if !common.TokenEqual(hello.Token, token) {
		logger.Warn("Token mismatch - authentication failed")
		sendErrorResponse(conn, proto.StatusAuthFail, "Authentication failed", logger)
		return
	}

	// Validate ports are within allowed range
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

	// Convert ports to int slice for registry
	ports := make([]int, len(hello.Ports))
	for i, p := range hello.Ports {
		ports[i] = int(p)
	}

	// Call registry.ReservePorts() atomically
	releaseFunc, err := registry.ReservePorts(ports)
	if err != nil {
		logger.Warn("Port reservation failed", zap.Error(err))
		sendErrorResponse(conn, proto.StatusPortInUse, "One or more ports are already in use", logger)
		return
	}

	// Track whether we need to release ports on error
	portsReserved := true
	defer func() {
		if portsReserved {
			releaseFunc()
		}
	}()

	// For each port, call net.Listen() on 127.0.0.1:port
	// If any Listen fails, release all and return PORT_IN_USE
	listeners := make(map[int]net.Listener)
	for _, port := range ports {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Warn("Failed to bind port", zap.Int("port", port), zap.Error(err))
			// Close any listeners we've already created
			for _, l := range listeners {
				l.Close()
			}
			sendErrorResponse(conn, proto.StatusPortInUse,
				fmt.Sprintf("Failed to bind port %d", port), logger)
			return
		}
		listeners[port] = listener
	}

	logger.Info("Ports bound successfully", zap.Ints("ports", ports))

	// Send HELLO_RESP with OK status
	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        proto.StatusOK,
		AcceptedPorts: hello.Ports,
		Message:       "Connection accepted",
	}

	// Set write deadline for HELLO_RESP
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", zap.Error(err))
		// Close listeners before returning
		for _, l := range listeners {
			l.Close()
		}
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write HELLO_RESP", zap.Error(err))
		// Close listeners before returning
		for _, l := range listeners {
			l.Close()
		}
		return
	}

	logger.Info("Sent HELLO_RESP with OK status")

	// Clear deadline after successful write
	if err := common.ClearDeadline(conn); err != nil {
		logger.Error("Failed to clear deadline", zap.Error(err))
		// Close listeners before returning
		for _, l := range listeners {
			l.Close()
		}
		return
	}

	// Create yamux.Server() session with keepalive config
	yamuxConfig := yamux.DefaultConfig()
	yamuxConfig.EnableKeepAlive = true
	yamuxConfig.KeepAliveInterval = 30 * time.Second
	yamuxConfig.ConnectionWriteTimeout = 10 * time.Second

	session, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		logger.Error("Failed to create yamux session", zap.Error(err))
		// Close listeners before returning
		for _, l := range listeners {
			l.Close()
		}
		return
	}

	logger.Info("Yamux session created")

	// Generate client ID for logging
	clientID := uuid.New().String()
	clientMeta := ClientMeta{
		ClientName: hello.Name,
		ClientID:   clientID,
	}

	// Start SOCKS5 server for each port and register session with all ports in registry
	for _, port := range ports {
		// Start SOCKS5 listener using socksManager
		socksListener, err := socksManager.StartListener(port, session)
		if err != nil {
			logger.Error("Failed to start SOCKS5 listener", zap.Int("port", port), zap.Error(err))
			// Close session and all listeners
			session.Close()
			for _, l := range listeners {
				l.Close()
			}
			return
		}

		// Close the TCP listener since SOCKS5 manager created its own
		if tcpListener, ok := listeners[port]; ok {
			tcpListener.Close()
		}

		// Register session with port in registry
		if err := registry.BindSession(port, session, socksListener, clientMeta); err != nil {
			logger.Error("Failed to bind session to port", zap.Int("port", port), zap.Error(err))
			// Close session and SOCKS listener
			session.Close()
			socksListener.Close()
			return
		}
	}

	// Ports are now successfully registered, don't release them in defer
	portsReserved = false

	logger.Info("Client session established",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name),
		zap.Ints("ports", ports))

	// Wait for session.CloseChan()
	<-session.CloseChan()

	logger.Info("Session closed, starting cleanup",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name))

	// Call registry.ReleasePorts() - this will close all SOCKS5 listeners
	registry.ReleasePorts(ports)

	// Log cleanup event
	logger.Info("Cleanup completed",
		zap.String("client_id", clientID),
		zap.String("client_name", hello.Name),
		zap.Ints("ports", ports))
}

// sendErrorResponse sends a HELLO_RESP with an error status
func sendErrorResponse(conn net.Conn, status uint8, message string, logger *zap.Logger) {
	resp := proto.HelloResp{
		Version:       proto.Version,
		Status:        status,
		AcceptedPorts: nil, // Error responses have zero accepted ports
		Message:       message,
	}

	// Set write deadline
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Error("Failed to set write deadline", zap.Error(err))
		return
	}

	if err := proto.WriteHelloResp(conn, resp); err != nil {
		logger.Error("Failed to write error response", zap.Uint8("status", status), zap.Error(err))
	}
}
