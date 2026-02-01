package client

import (
	"fmt"
	"net"
)

// AddressFilter validates and filters target addresses to prevent network abuse.
type AddressFilter struct {
	allowPrivate bool
	blockedNets  []*net.IPNet
}

// NewAddressFilter creates a new address filter with the specified configuration.
// blockedCIDRs is a list of CIDR blocks to block in addition to the default blocks.
func NewAddressFilter(allowPrivate bool, blockedCIDRs []string) (*AddressFilter, error) {
	af := &AddressFilter{
		allowPrivate: allowPrivate,
		blockedNets:  make([]*net.IPNet, 0, len(blockedCIDRs)),
	}

	// Parse custom blocked networks
	for _, cidr := range blockedCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR block %q: %w", cidr, err)
		}
		af.blockedNets = append(af.blockedNets, ipNet)
	}

	return af, nil
}

// IsAllowed checks if the given address is allowed to be connected to.
// Returns an error if the address is blocked, nil if allowed.
func (af *AddressFilter) IsAllowed(addr string) error {
	// Parse the address to extract host and port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address format: %w", err)
	}

	// Parse the IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// If not an IP, try to resolve it
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("failed to resolve hostname: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("hostname resolved to no addresses")
		}
		// Use the first resolved IP
		ip = ips[0]
	}

	// Check loopback addresses
	if af.isLoopback(ip) {
		return fmt.Errorf("loopback addresses are not allowed")
	}

	// Check link-local addresses
	if af.isLinkLocal(ip) {
		return fmt.Errorf("link-local addresses are not allowed")
	}

	// Check private networks (unless explicitly allowed)
	if !af.allowPrivate && af.isPrivateNetwork(ip) {
		return fmt.Errorf("private network addresses are not allowed")
	}

	// Check custom blocked networks
	if af.isInBlockedNetwork(ip) {
		return fmt.Errorf("address is in a blocked network")
	}

	return nil
}

// isLoopback checks if the IP is a loopback address.
// IPv4: 127.0.0.0/8
// IPv6: ::1
func (af *AddressFilter) isLoopback(ip net.IP) bool {
	return ip.IsLoopback()
}

// isLinkLocal checks if the IP is a link-local address.
// IPv4: 169.254.0.0/16
// IPv6: fe80::/10
func (af *AddressFilter) isLinkLocal(ip net.IP) bool {
	return ip.IsLinkLocalUnicast()
}

// isPrivateNetwork checks if the IP is in a private network range.
// IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
// IPv6: fc00::/7
func (af *AddressFilter) isPrivateNetwork(ip net.IP) bool {
	// Use the standard library's IsPrivate method (available in Go 1.17+)
	return ip.IsPrivate()
}

// isInBlockedNetwork checks if the IP is in any of the custom blocked networks.
func (af *AddressFilter) isInBlockedNetwork(ip net.IP) bool {
	for _, ipNet := range af.blockedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}
