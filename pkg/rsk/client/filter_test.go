package client

import (
	"net"
	"testing"
)

func TestNewAddressFilter(t *testing.T) {
	tests := []struct {
		name         string
		allowPrivate bool
		blockedCIDRs []string
		wantErr      bool
	}{
		{
			name:         "valid empty blocked list",
			allowPrivate: false,
			blockedCIDRs: []string{},
			wantErr:      false,
		},
		{
			name:         "valid single CIDR",
			allowPrivate: false,
			blockedCIDRs: []string{"203.0.113.0/24"},
			wantErr:      false,
		},
		{
			name:         "valid multiple CIDRs",
			allowPrivate: true,
			blockedCIDRs: []string{"203.0.113.0/24", "2001:db8::/32"},
			wantErr:      false,
		},
		{
			name:         "invalid CIDR format",
			allowPrivate: false,
			blockedCIDRs: []string{"invalid"},
			wantErr:      true,
		},
		{
			name:         "invalid CIDR in list",
			allowPrivate: false,
			blockedCIDRs: []string{"203.0.113.0/24", "not-a-cidr", "192.0.2.0/24"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAddressFilter(tt.allowPrivate, tt.blockedCIDRs)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAddressFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddressFilter_IsAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowPrivate bool
		blockedCIDRs []string
		addr         string
		wantErr      bool
		errContains  string
	}{
		// Loopback addresses
		{
			name:         "block IPv4 loopback",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "127.0.0.1:8080",
			wantErr:      true,
			errContains:  "loopback",
		},
		{
			name:         "block IPv4 loopback range",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "127.1.2.3:8080",
			wantErr:      true,
			errContains:  "loopback",
		},
		{
			name:         "block IPv6 loopback",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "[::1]:8080",
			wantErr:      true,
			errContains:  "loopback",
		},
		// Link-local addresses
		{
			name:         "block IPv4 link-local",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "169.254.1.1:8080",
			wantErr:      true,
			errContains:  "link-local",
		},
		{
			name:         "block IPv6 link-local",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "[fe80::1]:8080",
			wantErr:      true,
			errContains:  "link-local",
		},
		// Private networks (when not allowed)
		{
			name:         "block 10.0.0.0/8",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "10.1.2.3:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		{
			name:         "block 172.16.0.0/12",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "172.16.1.1:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		{
			name:         "block 172.31.255.255",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "172.31.255.255:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		{
			name:         "block 192.168.0.0/16",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "192.168.1.1:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		{
			name:         "block IPv6 private fc00::/7",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "[fc00::1]:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		{
			name:         "block IPv6 private fd00::/8",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "[fd00::1]:8080",
			wantErr:      true,
			errContains:  "private network",
		},
		// Private networks (when allowed)
		{
			name:         "allow 10.0.0.0/8 when flag set",
			allowPrivate: true,
			blockedCIDRs: []string{},
			addr:         "10.1.2.3:8080",
			wantErr:      false,
		},
		{
			name:         "allow 192.168.0.0/16 when flag set",
			allowPrivate: true,
			blockedCIDRs: []string{},
			addr:         "192.168.1.1:8080",
			wantErr:      false,
		},
		// Custom blocked networks
		{
			name:         "block custom CIDR",
			allowPrivate: false,
			blockedCIDRs: []string{"203.0.113.0/24"},
			addr:         "203.0.113.5:8080",
			wantErr:      true,
			errContains:  "blocked network",
		},
		{
			name:         "allow outside custom CIDR",
			allowPrivate: false,
			blockedCIDRs: []string{"203.0.113.0/24"},
			addr:         "203.0.114.5:8080",
			wantErr:      false,
		},
		{
			name:         "block custom IPv6 CIDR",
			allowPrivate: false,
			blockedCIDRs: []string{"2001:db8::/32"},
			addr:         "[2001:db8::1]:8080",
			wantErr:      true,
			errContains:  "blocked network",
		},
		// Public addresses (should be allowed)
		{
			name:         "allow public IPv4",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "8.8.8.8:53",
			wantErr:      false,
		},
		{
			name:         "allow public IPv6",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "[2001:4860:4860::8888]:53",
			wantErr:      false,
		},
		// Invalid addresses
		{
			name:         "invalid address format",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "invalid",
			wantErr:      true,
			errContains:  "invalid address format",
		},
		{
			name:         "missing port",
			allowPrivate: false,
			blockedCIDRs: []string{},
			addr:         "8.8.8.8",
			wantErr:      true,
			errContains:  "invalid address format",
		},
		// Edge cases
		{
			name:         "loopback takes precedence over allowPrivate",
			allowPrivate: true,
			blockedCIDRs: []string{},
			addr:         "127.0.0.1:8080",
			wantErr:      true,
			errContains:  "loopback",
		},
		{
			name:         "link-local takes precedence over allowPrivate",
			allowPrivate: true,
			blockedCIDRs: []string{},
			addr:         "169.254.1.1:8080",
			wantErr:      true,
			errContains:  "link-local",
		},
		{
			name:         "custom block takes precedence over allowPrivate",
			allowPrivate: true,
			blockedCIDRs: []string{"10.0.0.0/8"},
			addr:         "10.1.2.3:8080",
			wantErr:      true,
			errContains:  "blocked network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af, err := NewAddressFilter(tt.allowPrivate, tt.blockedCIDRs)
			if err != nil {
				t.Fatalf("NewAddressFilter() failed: %v", err)
			}

			err = af.IsAllowed(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsAllowed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" {
				if err == nil {
					t.Errorf("IsAllowed() expected error containing %q, got nil", tt.errContains)
				} else if !contains(err.Error(), tt.errContains) {
					t.Errorf("IsAllowed() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestAddressFilter_isLoopback(t *testing.T) {
	af := &AddressFilter{}

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 loopback 127.0.0.1", "127.0.0.1", true},
		{"IPv4 loopback 127.1.2.3", "127.1.2.3", true},
		{"IPv4 loopback 127.255.255.255", "127.255.255.255", true},
		{"IPv6 loopback ::1", "::1", true},
		{"IPv4 not loopback", "8.8.8.8", false},
		{"IPv6 not loopback", "2001:4860:4860::8888", false},
		{"IPv4 private not loopback", "10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			if got := af.isLoopback(ip); got != tt.want {
				t.Errorf("isLoopback(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestAddressFilter_isLinkLocal(t *testing.T) {
	af := &AddressFilter{}

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 link-local 169.254.0.1", "169.254.0.1", true},
		{"IPv4 link-local 169.254.255.255", "169.254.255.255", true},
		{"IPv6 link-local fe80::1", "fe80::1", true},
		{"IPv6 link-local fe80::abcd", "fe80::abcd", true},
		{"IPv4 not link-local", "8.8.8.8", false},
		{"IPv6 not link-local", "2001:4860:4860::8888", false},
		{"IPv4 private not link-local", "10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			if got := af.isLinkLocal(ip); got != tt.want {
				t.Errorf("isLinkLocal(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestAddressFilter_isPrivateNetwork(t *testing.T) {
	af := &AddressFilter{}

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// IPv4 private ranges
		{"10.0.0.0/8 start", "10.0.0.0", true},
		{"10.0.0.0/8 middle", "10.128.0.1", true},
		{"10.0.0.0/8 end", "10.255.255.255", true},
		{"172.16.0.0/12 start", "172.16.0.0", true},
		{"172.16.0.0/12 middle", "172.20.0.1", true},
		{"172.16.0.0/12 end", "172.31.255.255", true},
		{"192.168.0.0/16 start", "192.168.0.0", true},
		{"192.168.0.0/16 middle", "192.168.128.1", true},
		{"192.168.0.0/16 end", "192.168.255.255", true},
		// IPv6 private ranges
		{"fc00::/7 start", "fc00::1", true},
		{"fc00::/7 middle", "fd00::1", true},
		// Public addresses
		{"public IPv4", "8.8.8.8", false},
		{"public IPv6", "2001:4860:4860::8888", false},
		// Edge cases
		{"172.15.255.255 not private", "172.15.255.255", false},
		{"172.32.0.0 not private", "172.32.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			if got := af.isPrivateNetwork(ip); got != tt.want {
				t.Errorf("isPrivateNetwork(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestAddressFilter_isInBlockedNetwork(t *testing.T) {
	tests := []struct {
		name         string
		blockedCIDRs []string
		ip           string
		want         bool
	}{
		{
			name:         "empty blocked list",
			blockedCIDRs: []string{},
			ip:           "8.8.8.8",
			want:         false,
		},
		{
			name:         "IP in blocked network",
			blockedCIDRs: []string{"203.0.113.0/24"},
			ip:           "203.0.113.5",
			want:         true,
		},
		{
			name:         "IP not in blocked network",
			blockedCIDRs: []string{"203.0.113.0/24"},
			ip:           "203.0.114.5",
			want:         false,
		},
		{
			name:         "IP in one of multiple blocked networks",
			blockedCIDRs: []string{"203.0.113.0/24", "198.51.100.0/24"},
			ip:           "198.51.100.10",
			want:         true,
		},
		{
			name:         "IPv6 in blocked network",
			blockedCIDRs: []string{"2001:db8::/32"},
			ip:           "2001:db8::1",
			want:         true,
		},
		{
			name:         "IPv6 not in blocked network",
			blockedCIDRs: []string{"2001:db8::/32"},
			ip:           "2001:db9::1",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af, err := NewAddressFilter(false, tt.blockedCIDRs)
			if err != nil {
				t.Fatalf("NewAddressFilter() failed: %v", err)
			}

			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}

			if got := af.isInBlockedNetwork(ip); got != tt.want {
				t.Errorf("isInBlockedNetwork(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
