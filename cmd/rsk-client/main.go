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

type Config struct {
	serverAddr  string
	token       string
	port        int
	name        string
	dialTimeout time.Duration
}

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
		logger.Fatal("Failed to parse flags", zap.Error(err))
	}

	logger.Info("RSK Client starting",
		zap.String("server", cfg.serverAddr),
		zap.Int("port", cfg.port),
		zap.String("name", cfg.name))

	c := &client.Client{
		ServerAddr:     cfg.serverAddr,
		Token:          []byte(cfg.token),
		Ports:          []int{cfg.port},
		Name:           cfg.name,
		DialTimeout:    cfg.dialTimeout,
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

func parseFlags() (*Config, error) {
	conf := &Config{}

	pflag.StringVar(&conf.serverAddr, "server", "", "Server address (required)")
	pflag.StringVar(&conf.token, "token", "", "Authentication token (required)")
	pflag.IntVar(&conf.port, "port", 0, "Port to claim on the server (required)")
	pflag.StringVar(&conf.name, "name", "", "Client name for identification (optional, defaults to hostname)")
	pflag.DurationVar(&conf.dialTimeout, "dial-timeout", 15*time.Second, "Timeout for dialing target addresses")
	showVersion := pflag.BoolP("version", "v", false, "Show version information")

	pflag.Parse()

	if *showVersion {
		fmt.Println(version.GetFullVersion())
		os.Exit(0)
	}

	if conf.serverAddr == "" {
		return nil, fmt.Errorf("--server is required")
	}

	if conf.token == "" {
		return nil, fmt.Errorf("--token is required")
	}

	if conf.port == 0 {
		return nil, fmt.Errorf("--port is required")
	}

	if conf.port < 1 || conf.port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}

	if conf.name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			conf.name = "unknown"
		} else {
			conf.name = hostname
		}
	}

	return conf, nil
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
