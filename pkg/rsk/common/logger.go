package common

import (
	"log/slog"
	"os"
)

// NewLogger creates a logger with the specified level.
func NewLogger(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

// NewDefaultLogger creates a logger with Info level.
func NewDefaultLogger() *slog.Logger {
	return NewLogger(slog.LevelInfo)
}
