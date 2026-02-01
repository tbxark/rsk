package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/tbxark/rsk/pkg/rsk/client"
	"github.com/tbxark/rsk/pkg/rsk/version"
)

func main() {
	logger, err := initLogger()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfg, err := parseFlags()
	if err != nil {
		logger.Fatal("Failed to parse configuration", zap.Error(err))
	}

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Configuration validation failed", zap.Error(err))
	}

	logger.Info("RSK Client starting",
		zap.String("server", cfg.ServerAddr),
		zap.Int("port", cfg.Port),
		zap.String("name", cfg.Name),
		zap.Bool("token_validated", true),
		zap.Bool("allow_private_networks", cfg.AllowPrivateNetworks),
		zap.Strings("blocked_networks", cfg.BlockedNetworks))

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
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	if err := c.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("Client error", zap.Error(err))
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

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
