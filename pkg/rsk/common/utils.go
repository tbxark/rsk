package common

import (
	"crypto/rand"
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

// GenerateToken generates a cryptographically secure random token of the specified length.
// The token uses only alphanumeric characters (0-9, a-z, A-Z) to avoid shell escaping issues.
func GenerateToken(length int) (string, error) {
	if length < MinTokenLength {
		length = MinTokenLength
	}

	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const charsetLen = len(charset)

	// Generate random bytes
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// Convert to alphanumeric characters
	token := make([]byte, length)
	for i := 0; i < length; i++ {
		token[i] = charset[int(randomBytes[i])%charsetLen]
	}

	return string(token), nil
}
