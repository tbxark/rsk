package common

import "go.uber.org/zap"

// NewLogger creates a production logger with the specified level.
func NewLogger(level zap.AtomicLevel) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = level
	return config.Build()
}

// NewDefaultLogger creates a logger with Info level.
func NewDefaultLogger() (*zap.Logger, error) {
	return NewLogger(zap.NewAtomicLevelAt(zap.InfoLevel))
}
