package client

import (
	"fmt"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/common"
)

// Config holds client configuration.
type Config struct {
	ServerAddr           string
	Token                []byte
	Port                 int
	Name                 string
	DialTimeout          time.Duration
	AllowPrivateNetworks bool
	BlockedNetworks      []string
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate required fields
	if c.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
	if len(c.Token) == 0 {
		return fmt.Errorf("token is required")
	}
	if c.Port == 0 {
		return fmt.Errorf("port is required")
	}

	// Validate token strength
	if err := common.ValidateToken(c.Token); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Validate port range
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	// Validate CIDR format
	if err := ValidateCIDRs(c.BlockedNetworks); err != nil {
		return err
	}

	return nil
}

// ValidateCIDRs validates that all provided strings are valid CIDR blocks.
func ValidateCIDRs(cidrs []string) error {
	for _, cidr := range cidrs {
		hasSlash := false
		for _, ch := range cidr {
			if ch == '/' {
				hasSlash = true
				break
			}
		}
		if !hasSlash {
			return fmt.Errorf("invalid CIDR block %q: must be in CIDR notation (e.g., 10.0.0.0/8)", cidr)
		}
	}
	return nil
}

// ParseCommaSeparated splits a comma-separated string into a slice of trimmed strings.
func ParseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := []string{}
	for _, part := range splitByComma(s) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitByComma splits a string by comma.
func splitByComma(s string) []string {
	result := []string{}
	current := ""
	for _, ch := range s {
		if ch == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// trimSpace removes leading and trailing whitespace from a string.
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && isWhitespace(s[start]) {
		start++
	}

	for end > start && isWhitespace(s[end-1]) {
		end--
	}

	return s[start:end]
}

// isWhitespace checks if a byte is a whitespace character.
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
