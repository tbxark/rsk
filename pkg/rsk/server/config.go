package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tbxark/rsk/pkg/rsk/common"
)

// Config holds server configuration.
type Config struct {
	ListenAddr        string
	Token             []byte
	BindIP            string
	PortMin           int
	PortMax           int
	MaxClients        int
	MaxAuthFailures   int
	AuthBlockDuration time.Duration
	MaxConnsPerClient int
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate token strength
	if err := common.ValidateToken(c.Token); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Validate port range
	if c.PortMin < 1 || c.PortMin > 65535 {
		return fmt.Errorf("port-range minimum must be between 1 and 65535")
	}
	if c.PortMax < 1 || c.PortMax > 65535 {
		return fmt.Errorf("port-range maximum must be between 1 and 65535")
	}
	if c.PortMin > c.PortMax {
		return fmt.Errorf("port-range minimum must be less than or equal to maximum")
	}

	// Validate numeric parameters
	if c.MaxClients <= 0 {
		return fmt.Errorf("max-clients must be greater than 0")
	}
	if c.MaxAuthFailures <= 0 {
		return fmt.Errorf("max-auth-failures must be greater than 0")
	}
	if c.AuthBlockDuration <= 0 {
		return fmt.Errorf("auth-block-duration must be greater than 0")
	}
	if c.MaxConnsPerClient <= 0 {
		return fmt.Errorf("max-connections-per-client must be greater than 0")
	}

	return nil
}

// ParsePortRange parses a port range string in the format "min-max".
func ParsePortRange(portRange string) (int, int, error) {
	parts := strings.Split(portRange, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port-range format, expected min-max")
	}

	portMin, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port-range minimum: %w", err)
	}

	portMax, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port-range maximum: %w", err)
	}

	return portMin, portMax, nil
}
