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
	"github.com/tbxark/rsk/pkg/rsk/common"
	"github.com/tbxark/rsk/pkg/rsk/server"
	"github.com/tbxark/rsk/pkg/rsk/version"
	"go.uber.org/zap"
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

	logger.Info("RSK Server starting")

	cfg, err := parseFlags()
	if err != nil {
		logger.Fatal("Failed to parse configuration", zap.Error(err))
	}

	if err := cfg.Validate(); err != nil {
		logger.Fatal("Configuration validation failed", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("listen", cfg.ListenAddr),
		zap.String("bind", cfg.BindIP),
		zap.Int("port_min", cfg.PortMin),
		zap.Int("port_max", cfg.PortMax),
		zap.Int("max_clients", cfg.MaxClients),
		zap.Int("max_auth_failures", cfg.MaxAuthFailures),
		zap.Duration("auth_block_duration", cfg.AuthBlockDuration),
		zap.Int("max_connections_per_client", cfg.MaxConnsPerClient),
		zap.Bool("token_validated", true))

	srv := server.NewServer(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errChan <- err
		}
	}()

	select {
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	case err := <-errChan:
		logger.Fatal("Server error", zap.Error(err))
	}

	logger.Info("RSK Server stopped")
}

func parseFlags() (*server.Config, error) {
	var (
		listenAddr        string
		token             string
		bindIP            string
		portRange         string
		maxClients        int
		maxAuthFailures   int
		authBlockDuration time.Duration
		maxConnsPerClient int
		showVersion       bool
	)

	pflag.StringVar(&listenAddr, "listen", ":7000", "Address to listen for client connections")
	pflag.StringVar(&token, "token", "", "Authentication token (required)")
	pflag.StringVar(&bindIP, "bind", "127.0.0.1", "IP address to bind SOCKS5 listeners")
	pflag.StringVar(&portRange, "port-range", "20000-40000", "Allowed port range for SOCKS5 listeners (format: min-max)")
	pflag.IntVar(&maxClients, "max-clients", 100, "Maximum number of concurrent client connections")
	pflag.IntVar(&maxAuthFailures, "max-auth-failures", 5, "Maximum authentication failures before blocking IP")
	pflag.DurationVar(&authBlockDuration, "auth-block-duration", 5*time.Minute, "Duration to block IP after max auth failures")
	pflag.IntVar(&maxConnsPerClient, "max-connections-per-client", 100, "Maximum SOCKS5 connections per client")
	pflag.BoolVarP(&showVersion, "version", "v", false, "Show version information")

	pflag.Parse()

	if showVersion {
		fmt.Println(version.GetFullVersion())
		os.Exit(0)
	}

	// Auto-generate token if not provided
	if token == "" {
		generatedToken, err := common.GenerateToken(common.MinTokenLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate token: %w", err)
		}
		token = generatedToken
		fmt.Printf("\n⚠️  No token provided. Auto-generated secure token:\n")
		fmt.Printf("   %s\n", generatedToken)
		fmt.Printf("\n💡 Save this token! Use it with: --token=\"%s\"\n\n", generatedToken)
	}

	// Parse port range
	portMin, portMax, err := server.ParsePortRange(portRange)
	if err != nil {
		return nil, err
	}

	return &server.Config{
		ListenAddr:        listenAddr,
		Token:             []byte(token),
		BindIP:            bindIP,
		PortMin:           portMin,
		PortMax:           portMax,
		MaxClients:        maxClients,
		MaxAuthFailures:   maxAuthFailures,
		AuthBlockDuration: authBlockDuration,
		MaxConnsPerClient: maxConnsPerClient,
	}, nil
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
