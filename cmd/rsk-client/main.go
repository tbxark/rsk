package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

func main() {
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("RSK Client starting")
	// TODO: Implement client logic
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
