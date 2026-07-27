package goforge

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerFromContextOrUsesFallback(t *testing.T) {
	fallback := slog.New(slog.DiscardHandler)

	if logger := LoggerFromContextOr(t.Context(), fallback); logger != fallback {
		t.Fatal("expected fallback logger")
	}
}

func TestLoggerFromContextOrPrefersContextLogger(t *testing.T) {
	fallback := slog.New(slog.DiscardHandler)
	contextLogger := slog.New(slog.DiscardHandler)
	ctx := WithLogger(context.Background(), contextLogger)

	if logger := LoggerFromContextOr(ctx, fallback); logger != contextLogger {
		t.Fatal("expected context logger")
	}
}

func TestLoggerFromContextOrUsesFallbackForNilContextLogger(t *testing.T) {
	fallback := slog.New(slog.DiscardHandler)
	ctx := WithLogger(context.Background(), nil)

	if logger := LoggerFromContextOr(ctx, fallback); logger != fallback {
		t.Fatal("expected fallback logger")
	}
}
