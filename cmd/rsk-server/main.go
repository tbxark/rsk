package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/pflag"
	"github.com/tbxark/rsk/pkg/rsk/server"
	"github.com/tbxark/rsk/pkg/rsk/version"
	"go.uber.org/zap"
)

type Config struct {
	listenAddr string
	token      string
	bindIP     string
	portMin    int
	portMax    int
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

	logger.Info("RSK Server starting")

	cfg, err := parseFlags()
	if err != nil {
		logger.Fatal("Failed to parse flags", zap.Error(err))
	}

	logger.Info("Configuration loaded",
		zap.String("listen", cfg.listenAddr),
		zap.String("bind", cfg.bindIP),
		zap.Int("port_min", cfg.portMin),
		zap.Int("port_max", cfg.portMax))

	srv := server.NewServer(
		cfg.listenAddr,
		cfg.bindIP,
		[]byte(cfg.token),
		cfg.portMin,
		cfg.portMax,
		logger,
	)

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

func parseFlags() (*Config, error) {
	conf := &Config{}

	pflag.StringVar(&conf.listenAddr, "listen", ":7000", "Address to listen for client connections")
	pflag.StringVar(&conf.token, "token", "", "Authentication token (required)")
	pflag.StringVar(&conf.bindIP, "bind", "127.0.0.1", "IP address to bind SOCKS5 listeners")
	portRange := pflag.String("port-range", "20000-40000", "Allowed port range for SOCKS5 listeners (format: min-max)")
	showVersion := pflag.BoolP("version", "v", false, "Show version information")

	pflag.Parse()

	if *showVersion {
		fmt.Println(version.GetFullVersion())
		os.Exit(0)
	}

	if conf.token == "" {
		return nil, fmt.Errorf("--token is required")
	}

	parts := strings.Split(*portRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid port-range format, expected min-max")
	}

	var err error
	conf.portMin, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid port-range minimum: %w", err)
	}

	conf.portMax, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid port-range maximum: %w", err)
	}

	if conf.portMin < 1 || conf.portMin > 65535 {
		return nil, fmt.Errorf("port-range minimum must be between 1 and 65535")
	}
	if conf.portMax < 1 || conf.portMax > 65535 {
		return nil, fmt.Errorf("port-range maximum must be between 1 and 65535")
	}
	if conf.portMin > conf.portMax {
		return nil, fmt.Errorf("port-range minimum must be less than or equal to maximum")
	}

	return conf, nil
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
