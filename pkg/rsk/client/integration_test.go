package client

import (
	"testing"

	"go.uber.org/zap"
)

// TestAddressFilterIntegration tests the integration of address filter with the client
func TestAddressFilterIntegration(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name             string
		allowPrivate     bool
		blockedNetworks  []string
		targetAddr       string
		shouldBeBlocked  bool
		expectedErrorMsg string
	}{
		{
			name:             "block loopback address",
			allowPrivate:     false,
			blockedNetworks:  nil,
			targetAddr:       "127.0.0.1:8080",
			shouldBeBlocked:  true,
			expectedErrorMsg: "loopback addresses are not allowed",
		},
		{
			name:             "block link-local address",
			allowPrivate:     false,
			blockedNetworks:  nil,
			targetAddr:       "169.254.1.1:8080",
			shouldBeBlocked:  true,
			expectedErrorMsg: "link-local addresses are not allowed",
		},
		{
			name:             "block private network by default",
			allowPrivate:     false,
			blockedNetworks:  nil,
			targetAddr:       "192.168.1.1:8080",
			shouldBeBlocked:  true,
			expectedErrorMsg: "private network addresses are not allowed",
		},
		{
			name:             "allow private network when flag set",
			allowPrivate:     true,
			blockedNetworks:  nil,
			targetAddr:       "192.168.1.1:8080",
			shouldBeBlocked:  false,
			expectedErrorMsg: "",
		},
		{
			name:             "block custom network",
			allowPrivate:     true,
			blockedNetworks:  []string{"203.0.113.0/24"},
			targetAddr:       "203.0.113.50:8080",
			shouldBeBlocked:  true,
			expectedErrorMsg: "address is in a blocked network",
		},
		{
			name:             "allow public address",
			allowPrivate:     false,
			blockedNetworks:  nil,
			targetAddr:       "8.8.8.8:53",
			shouldBeBlocked:  false,
			expectedErrorMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create address filter
			filter, err := NewAddressFilter(tt.allowPrivate, tt.blockedNetworks)
			if err != nil {
				t.Fatalf("Failed to create address filter: %v", err)
			}

			// Test IsAllowed
			err = filter.IsAllowed(tt.targetAddr)

			if tt.shouldBeBlocked {
				if err == nil {
					t.Errorf("Expected address to be blocked, but it was allowed")
				} else if err.Error() != tt.expectedErrorMsg {
					t.Errorf("Expected error message %q, got %q", tt.expectedErrorMsg, err.Error())
				} else {
					// Log the rejection as the handleStream function would
					logger.Warn("Target address blocked by filter",
						zap.String("addr", tt.targetAddr),
						zap.Error(err))
				}
			} else {
				if err != nil {
					t.Errorf("Expected address to be allowed, but it was blocked: %v", err)
				}
			}
		})
	}
}

// TestAddressFilterCreationErrors tests error handling during filter creation
func TestAddressFilterCreationErrors(t *testing.T) {
	tests := []struct {
		name            string
		blockedNetworks []string
		expectError     bool
	}{
		{
			name:            "valid CIDR",
			blockedNetworks: []string{"10.0.0.0/8"},
			expectError:     false,
		},
		{
			name:            "invalid CIDR format",
			blockedNetworks: []string{"not-a-cidr"},
			expectError:     true,
		},
		{
			name:            "invalid CIDR in list",
			blockedNetworks: []string{"10.0.0.0/8", "invalid", "192.168.0.0/16"},
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAddressFilter(false, tt.blockedNetworks)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
