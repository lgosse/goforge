package forgesentry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	slogSentryContext = "slog"
	errorAttrKey      = "error"
)

func NewLogger(h slog.Handler) *slog.Logger {
	if h == nil {
		h = slog.NewJSONHandler(io.Discard, nil)
	}

	return slog.New(NewSentryHandler(h))
}

type sentryHandler struct {
	next   slog.Handler
	attrs  []slog.Attr
	groups []string
}

func NewSentryHandler(next slog.Handler) slog.Handler {
	return &sentryHandler{next: next}
}

func (h *sentryHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *sentryHandler) Handle(ctx context.Context, record slog.Record) error {
	handleErr := h.next.Handle(ctx, record)
	if record.Level < slog.LevelError {
		return handleErr
	}

	attrs, exception := h.sentryAttrs(record)
	if exception == nil {
		exception = errors.New(record.Message)
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub = hub.Clone()
	if scope := hub.Scope(); scope != nil {
		scope.SetContext(slogSentryContext, sentry.Context{
			"level":   record.Level.String(),
			"message": record.Message,
			"attrs":   attrs,
		})
		scope.SetExtras(attrs)
	}
	hub.CaptureException(exception)

	return handleErr
}

func (h *sentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := h.next.WithAttrs(attrs)
	cloned := h.clone(next)
	cloned.attrs = append(cloned.attrs, collectAttrs(attrs, h.groups, nil)...)
	return cloned
}

func (h *sentryHandler) WithGroup(name string) slog.Handler {
	next := h.next.WithGroup(name)
	cloned := h.clone(next)
	if name != "" {
		cloned.groups = append(cloned.groups, name)
	}
	return cloned
}

func (h *sentryHandler) clone(next slog.Handler) *sentryHandler {
	return &sentryHandler{
		next:   next,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: append([]string(nil), h.groups...),
	}
}

func (h *sentryHandler) sentryAttrs(record slog.Record) (map[string]any, error) {
	collected := make(map[string]any)
	var exception error

	for _, attr := range h.attrs {
		addSentryAttr(collected, attr, nil, &exception)
	}

	record.Attrs(func(attr slog.Attr) bool {
		addSentryAttr(collected, attr, h.groups, &exception)
		return true
	})

	return collected, exception
}

func collectAttrs(attrs []slog.Attr, groups []string, exception *error) []slog.Attr {
	collected := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		collected = append(collected, groupAttr(attr, groups))
		if exception != nil {
			addSentryAttr(nil, attr, groups, exception)
		}
	}
	return collected
}

func groupAttr(attr slog.Attr, groups []string) slog.Attr {
	if len(groups) == 0 {
		return attr
	}

	for idx := len(groups) - 1; idx >= 0; idx-- {
		attr = slog.Group(groups[idx], attr)
	}
	return attr
}

func addSentryAttr(attrs map[string]any, attr slog.Attr, groups []string, exception *error) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	keyParts := append(append([]string(nil), groups...), attr.Key)
	if attr.Value.Kind() == slog.KindGroup {
		for _, groupedAttr := range attr.Value.Group() {
			addSentryAttr(attrs, groupedAttr, keyParts, exception)
		}
		return
	}

	value := sentryAttrValue(attr.Value)
	if attr.Key == errorAttrKey && exception != nil {
		if err, ok := attr.Value.Any().(error); ok {
			*exception = err
		} else if value != nil {
			*exception = fmt.Errorf("%v", value)
		}
	}

	if attrs != nil {
		attrs[strings.Join(keyParts, ".")] = value
	}
}

func sentryAttrValue(value slog.Value) any {
	value = value.Resolve()

	switch value.Kind() {
	case slog.KindAny:
		if err, ok := value.Any().(error); ok {
			return err.Error()
		}
		return value.Any()
	case slog.KindBool:
		return value.Bool()
	case slog.KindDuration:
		return value.Duration().String()
	case slog.KindFloat64:
		return value.Float64()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindString:
		return value.String()
	case slog.KindTime:
		return value.Time().Format(time.RFC3339Nano)
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindLogValuer:
		return sentryAttrValue(value.Resolve())
	case slog.KindGroup:
		group := make(map[string]any)
		for _, attr := range value.Group() {
			addSentryAttr(group, attr, nil, nil)
		}
		return group
	default:
		return fmt.Sprint(value.Any())
	}
}
