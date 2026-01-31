package client

import (
	"io"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/tbxark/rsk/internal/common"
	"github.com/tbxark/rsk/internal/proto"
)

// Package client implements the RSK client components including
// the main client and stream handler.

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
