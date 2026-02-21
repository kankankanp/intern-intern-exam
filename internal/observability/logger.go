package observability

import (
	"log/slog"
	"os"
)

// NewLogger creates a new structured logger
func NewLogger(service string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Use JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stdout, opts)

	logger := slog.New(handler).With(
		slog.String("service", service),
	)

	return logger
}
