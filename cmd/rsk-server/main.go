package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/pflag"
	"github.com/tbxark/rsk/internal/server"
	"go.uber.org/zap"
)

// Config holds the parsed command-line configuration
type Config struct {
	listenAddr string
	token      string
	bindIP     string
	portMin    int
	portMax    int
}

// parseFlags parses command-line flags and returns the configuration
func parseFlags() (*Config, error) {
	cfg := &Config{}

	// Define flags
	pflag.StringVar(&cfg.listenAddr, "listen", ":7000", "Address to listen for client connections")
	pflag.StringVar(&cfg.token, "token", "", "Authentication token (required)")
	pflag.StringVar(&cfg.bindIP, "bind", "127.0.0.1", "IP address to bind SOCKS5 listeners")
	portRange := pflag.String("port-range", "20000-40000", "Allowed port range for SOCKS5 listeners (format: min-max)")

	pflag.Parse()

	// Validate required parameters
	if cfg.token == "" {
		return nil, fmt.Errorf("--token is required")
	}

	// Parse port range
	parts := strings.Split(*portRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port-range format, expected min-max")
	}

	var err error
	cfg.portMin, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid port-range minimum: %w", err)
	}

	cfg.portMax, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid port-range maximum: %w", err)
	}

	// Validate port range
	if cfg.portMin < 1 || cfg.portMin > 65535 {
		return nil, fmt.Errorf("port-range minimum must be between 1 and 65535")
	}
	if cfg.portMax < 1 || cfg.portMax > 65535 {
		return nil, fmt.Errorf("port-range maximum must be between 1 and 65535")
	}
	if cfg.portMin > cfg.portMax {
		return nil, fmt.Errorf("port-range minimum must be less than or equal to maximum")
	}

	return cfg, nil
}

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("RSK Server starting")

	// Parse CLI flags
	cfg, err := parseFlags()
	if err != nil {
		logger.Fatal("Failed to parse flags", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("listen", cfg.listenAddr),
		zap.String("bind", cfg.bindIP),
		zap.Int("port_min", cfg.portMin),
		zap.Int("port_max", cfg.portMax))

	// Create Server instance
	srv := server.NewServer(
		cfg.listenAddr,
		cfg.bindIP,
		[]byte(cfg.token),
		cfg.portMin,
		cfg.portMax,
		logger,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	case err := <-errChan:
		logger.Fatal("Server error", zap.Error(err))
	}

	logger.Info("RSK Server stopped")
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
