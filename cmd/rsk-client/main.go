package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/version"
)

func main() {
	logger := slog.Default()

	cfg, err := parseFlags()
	if err != nil {
		logger.Error("Failed to parse configuration", "error", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	logger.Info("RSK Client starting",
		"server", cfg.ServerAddr,
		"port", cfg.Port,
		"name", cfg.Name,
		"token_validated", true,
		"allow_private_networks", cfg.AllowPrivateNetworks,
		"blocked_networks", cfg.BlockedNetworks)

	c := &client.Client{
		Config:         cfg,
		ReconnectDelay: 2 * time.Second,
		Logger:         logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received signal, shutting down", "signal", sig.String())
		cancel()
	}()

	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Client error", "error", err)
		os.Exit(1)
	}

	logger.Info("RSK Client stopped")
}

func parseFlags() (*client.Config, error) {
	var (
		serverAddr           string
		token                string
		port                 int
		name                 string
		dialTimeout          time.Duration
		allowPrivateNetworks bool
		blockedNetworksStr   string
		showVersion          bool
	)

	pflag.StringVar(&serverAddr, "server", "", "Server address (required)")
	pflag.StringVar(&token, "token", "", "Authentication token (required)")
	pflag.IntVar(&port, "port", 0, "Port to claim on the server (required)")
	pflag.StringVar(&name, "name", "", "Client name for identification (optional, defaults to hostname)")
	pflag.DurationVar(&dialTimeout, "dial-timeout", 15*time.Second, "Timeout for dialing target addresses")
	pflag.BoolVar(&allowPrivateNetworks, "allow-private-networks", false, "Allow connections to private IP ranges")
	pflag.StringVar(&blockedNetworksStr, "blocked-networks", "", "Additional CIDR blocks to block (comma-separated)")
	pflag.BoolVarP(&showVersion, "version", "v", false, "Show version information")

	pflag.Parse()

	if showVersion {
		fmt.Println(version.GetFullVersion())
		os.Exit(0)
	}

	// Validate required fields
	if serverAddr == "" {
		return nil, fmt.Errorf("--server is required")
	}
	if token == "" {
		return nil, fmt.Errorf("--token is required")
	}
	if port == 0 {
		return nil, fmt.Errorf("--port is required")
	}

	// Default name to hostname
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			name = "unknown"
		} else {
			name = hostname
		}
	}

	// Parse blocked networks
	var blockedNetworks []string
	if blockedNetworksStr != "" {
		blockedNetworks = client.ParseCommaSeparated(blockedNetworksStr)
	}

	return &client.Config{
		ServerAddr:           serverAddr,
		Token:                []byte(token),
		Port:                 port,
		Name:                 name,
		DialTimeout:          dialTimeout,
		AllowPrivateNetworks: allowPrivateNetworks,
		BlockedNetworks:      blockedNetworks,
	}, nil
}
