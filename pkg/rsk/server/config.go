package server

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/tbxark/rsk/pkg/rsk/common"
)

// Config holds server configuration.
type Config struct {
	ListenAddr        string        `validate:"required"`
	Token             []byte        `validate:"required,min=16"`
	BindIP            string        `validate:"required,ip"`
	PortMin           int           `validate:"required,min=1,max=65535"`
	PortMax           int           `validate:"required,min=1,max=65535,gtefield=PortMin"`
	MaxClients        int           `validate:"required,min=1"`
	MaxAuthFailures   int           `validate:"required,min=1"`
	AuthBlockDuration time.Duration `validate:"required,min=1ms"`
	MaxConnsPerClient int           `validate:"required,min=1"`
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
