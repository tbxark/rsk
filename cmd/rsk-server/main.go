package main

import (
	"fmt"
	"os"

	_ "github.com/spf13/pflag"
	"go.uber.org/zap"
)

func main() {
	logger, err := initLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("RSK Server starting")
	// TODO: Implement server logic
}

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}
