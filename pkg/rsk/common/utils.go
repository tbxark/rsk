package common

import (
	"crypto/subtle"
	"net"
	"time"
)

// TokenEqual performs constant-time comparison of two tokens.
func TokenEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

func SetReadDeadline(conn net.Conn, timeout time.Duration) error {
	return conn.SetReadDeadline(time.Now().Add(timeout))
}

func ClearDeadline(conn net.Conn) error {
	return conn.SetDeadline(time.Time{})
}
