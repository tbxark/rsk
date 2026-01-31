package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/tbxark/rsk/internal/client"
)

func main() {
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Parse CLI flags (Requirements: 13.5, 13.6, 13.7, 13.8, 13.9)
	cfg, err := parseFlags()
	if err != nil {
		logger.Error("Failed to parse flags", zap.Error(err))
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	logger.Info("RSK Client starting",
		zap.String("server", cfg.serverAddr),
		zap.Ints("ports", cfg.ports),
		zap.String("name", cfg.name))

	// Create Client instance
	c := &client.Client{
		ServerAddr:     cfg.serverAddr,
		Token:          []byte(cfg.token),
		Ports:          cfg.ports,
		Name:           cfg.name,
		DialTimeout:    cfg.dialTimeout,
		ReconnectDelay: 2 * time.Second,
		Logger:         logger,
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Run client
	if err := c.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("Client error", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("RSK Client stopped")
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}

// clientConfig holds the parsed CLI configuration
type clientConfig struct {
	serverAddr  string
	token       string
	ports       []int
	name        string
	dialTimeout time.Duration
}

// parseFlags parses and validates CLI flags
// Requirements: 13.5, 13.6, 13.7, 13.8, 13.9
func parseFlags() (*clientConfig, error) {
	cfg := &clientConfig{}

	// Define flags
	serverFlag := pflag.String("server", "", "Server address (required)")
	tokenFlag := pflag.String("token", "", "Authentication token (required)")
	portsFlag := pflag.String("ports", "", "Comma-separated list of ports to claim (required)")
	nameFlag := pflag.String("name", "", "Client name for identification (optional, defaults to hostname)")
	dialTimeoutFlag := pflag.Duration("dial-timeout", 15*time.Second, "Timeout for dialing target addresses")

	pflag.Parse()

	// Validate required parameters
	if *serverFlag == "" {
		return nil, fmt.Errorf("--server is required")
	}
	cfg.serverAddr = *serverFlag

	if *tokenFlag == "" {
		return nil, fmt.Errorf("--token is required")
	}
	cfg.token = *tokenFlag

	if *portsFlag == "" {
		return nil, fmt.Errorf("--ports is required")
	}

	// Parse comma-separated ports
	portStrs := strings.Split(*portsFlag, ",")
	cfg.ports = make([]int, 0, len(portStrs))
	for _, ps := range portStrs {
		ps = strings.TrimSpace(ps)
		if ps == "" {
			continue
		}
		port, err := strconv.Atoi(ps)
		if err != nil {
			return nil, fmt.Errorf("invalid port '%s': %w", ps, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of range (1-65535)", port)
		}
		cfg.ports = append(cfg.ports, port)
	}

	if len(cfg.ports) == 0 {
		return nil, fmt.Errorf("at least one port must be specified")
	}

	// Set name (default to hostname if not provided)
	if *nameFlag == "" {
		hostname, err := os.Hostname()
		if err != nil {
			cfg.name = "unknown"
		} else {
			cfg.name = hostname
		}
	} else {
		cfg.name = *nameFlag
	}

	cfg.dialTimeout = *dialTimeoutFlag

	return cfg, nil
}
