package client

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/tbxark/rsk/pkg/rsk/common"
)

// Config holds client configuration.
type Config struct {
	ServerAddr           string        `validate:"required"`
	Token                []byte        `validate:"required,min=16"`
	Port                 int           `validate:"required,min=1,max=65535"`
	Name                 string        `validate:"required"`
	DialTimeout          time.Duration `validate:"required,min=1ms"`
	AllowPrivateNetworks bool
	BlockedNetworks      []string
}

var validate = validator.New()

// Validate validates the configuration.
func (c *Config) Validate() error {
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	if err := common.ValidateToken(c.Token); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if err := ValidateCIDRs(c.BlockedNetworks); err != nil {
		return err
	}

	return nil
}

// ValidateCIDRs validates that all provided strings are valid CIDR blocks.
func ValidateCIDRs(cidrs []string) error {
	for _, cidr := range cidrs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid CIDR block %q: %w", cidr, err)
		}
	}
	return nil
}

// ParseCommaSeparated splits a comma-separated string into trimmed strings.
func ParseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
