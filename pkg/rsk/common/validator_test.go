package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name      string
		token     []byte
		expectErr bool
	}{
		{
			name:      "valid token - exactly 16 bytes",
			token:     []byte("1234567890123456"),
			expectErr: false,
		},
		{
			name:      "valid token - longer than 16 bytes",
			token:     []byte("12345678901234567890"),
			expectErr: false,
		},
		{
			name:      "invalid token - 15 bytes",
			token:     []byte("123456789012345"),
			expectErr: true,
		},
		{
			name:      "invalid token - empty",
			token:     []byte(""),
			expectErr: true,
		},
		{
			name:      "invalid token - nil",
			token:     nil,
			expectErr: true,
		},
		{
			name:      "invalid token - 1 byte",
			token:     []byte("a"),
			expectErr: true,
		},
		{
			name:      "invalid token - 8 bytes",
			token:     []byte("12345678"),
			expectErr: true,
		},
		{
			name:      "valid token - 32 bytes",
			token:     []byte("12345678901234567890123456789012"),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToken(tt.token)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "token too short")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
