package otel

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/trace"
)

func newLogHandler(config Config, provider *Runtime) slog.Handler {
	handlers := make([]slog.Handler, 0, 2)
	if config.Logs.ConsoleEnabled {
		options := &slog.HandlerOptions{
			AddSource: config.Logs.AddSource,
			Level:     config.Logs.Level,
		}
		if config.Logs.ConsoleFormat == ConsoleFormatJSON {
			handlers = append(handlers, slog.NewJSONHandler(config.Logs.ConsoleWriter, options))
		} else {
			handlers = append(handlers, slog.NewTextHandler(config.Logs.ConsoleWriter, options))
		}
	}
	if config.Logs.Enabled {
		handlers = append(handlers, &levelHandler{
			level: config.Logs.Level,
			next: otelslog.NewHandler(
				config.ServiceName,
				otelslog.WithLoggerProvider(provider.LoggerProvider()),
				otelslog.WithVersion(config.ServiceVersion),
				otelslog.WithSource(config.Logs.AddSource),
			),
		})
	}

	return &traceContextHandler{next: fanoutHandler(handlers)}
}

type levelHandler struct {
	level slog.Leveler
	next  slog.Handler
}

func (h *levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level.Level() && h.next.Enabled(ctx, level)
}

func (h *levelHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.next.Handle(ctx, record)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{level: h.level, next: h.next.WithAttrs(attrs)}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{level: h.level, next: h.next.WithGroup(name)}
}

type traceContextHandler struct {
	next slog.Handler
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanContext.TraceID().String()),
			slog.String("span_id", spanContext.SpanID().String()),
		)
	}

	return h.next.Handle(ctx, record)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{next: h.next.WithGroup(name)}
}

type fanoutHandler []slog.Handler

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h {
		if handler.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var result error
	for _, handler := range h {
		if handler.Enabled(ctx, record.Level) {
			result = errors.Join(result, handler.Handle(ctx, record.Clone()))
		}
	}

	return result
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(fanoutHandler, 0, len(h))
	for _, handler := range h {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}

	return handlers
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make(fanoutHandler, 0, len(h))
	for _, handler := range h {
		handlers = append(handlers, handler.WithGroup(name))
	}

	return handlers
}
