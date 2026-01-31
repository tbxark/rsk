package common

import (
	"crypto/subtle"
	"net"
	"time"
)

// Package common provides shared utilities for both server and client,
// including token comparison and deadline helpers.

// TokenEqual performs constant-time comparison of two token byte slices.
// This prevents timing attacks during authentication.
// Returns true if tokens are equal, false otherwise.
func TokenEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// SetReadDeadline sets a read deadline on the connection for the specified timeout duration.
// This is used to prevent indefinite blocking on read operations.
// Returns an error if setting the deadline fails.
func SetReadDeadline(conn net.Conn, timeout time.Duration) error {
	return conn.SetReadDeadline(time.Now().Add(timeout))
}

// ClearDeadline clears all deadlines on the connection.
// This should be called after temporary deadline operations complete successfully.
// Returns an error if clearing the deadline fails.
func ClearDeadline(conn net.Conn) error {
	return conn.SetDeadline(time.Time{})
}
