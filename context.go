package goforge

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// WithLogger adds a *log/slog.Logger to the context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// LoggerFromContext retrieves the logger from the context.
// If no logger is found, it returns a default logger.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok {
		return logger
	}

	return slog.New(slog.Default().Handler())
}
