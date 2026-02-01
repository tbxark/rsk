package common

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []byte
		b        []byte
		expected bool
	}{
		{
			name:     "equal tokens",
			a:        []byte("secret123"),
			b:        []byte("secret123"),
			expected: true,
		},
		{
			name:     "different tokens",
			a:        []byte("secret123"),
			b:        []byte("secret456"),
			expected: false,
		},
		{
			name:     "different lengths",
			a:        []byte("short"),
			b:        []byte("muchlongertoken"),
			expected: false,
		},
		{
			name:     "empty tokens",
			a:        []byte(""),
			b:        []byte(""),
			expected: true,
		},
		{
			name:     "one empty",
			a:        []byte("token"),
			b:        []byte(""),
			expected: false,
		},
		{
			name:     "nil tokens",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil",
			a:        []byte("token"),
			b:        nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetReadDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer func() {
		_ = server.Close()
	}()
	defer func() {
		_ = client.Close()
	}()

	timeout := 100 * time.Millisecond
	err := SetReadDeadline(client, timeout)
	require.NoError(t, err)

	buf := make([]byte, 10)
	start := time.Now()
	_, err = client.Read(buf)
	elapsed := time.Since(start)

	assert.Error(t, err)
	netErr, ok := err.(net.Error)
	require.True(t, ok, "error should be a net.Error")
	assert.True(t, netErr.Timeout(), "error should be a timeout")

	assert.True(t, elapsed >= timeout, "should wait at least the timeout duration")
	assert.True(t, elapsed < timeout*2, "should not wait much longer than timeout")
}

func TestClearDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer func() {
		_ = server.Close()
	}()
	defer func() {
		_ = client.Close()
	}()

	err := SetReadDeadline(client, 100*time.Millisecond)
	require.NoError(t, err)

	err = ClearDeadline(client)
	require.NoError(t, err)

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = server.Write([]byte("test"))
	}()

	// Try to read - should succeed without timeout
	buf := make([]byte, 10)
	n, err := client.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, "test", string(buf[:n]))
}
