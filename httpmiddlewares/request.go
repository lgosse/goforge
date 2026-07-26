package httpmiddlewares

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// elapsedTracker implements slog.LogValuer
type elapsedTracker struct {
	start time.Time
}

type requestPatternTracker struct {
	request *http.Request
}

type requestRouteAttrsTracker struct {
	request *http.Request
}

// LogValue is called by slog right when the log is being written.
func (e elapsedTracker) LogValue() slog.Value {
	return slog.DurationValue(time.Since(e.start))
}

func (t requestPatternTracker) LogValue() slog.Value {
	if t.request == nil {
		return slog.StringValue("")
	}

	return slog.StringValue(t.request.Pattern)
}

func (t requestRouteAttrsTracker) LogValue() slog.Value {
	if t.request == nil {
		return slog.GroupValue(slog.String("route", ""))
	}

	attrs := []slog.Attr{slog.Any("route", requestPatternTracker{request: t.request})}
	for _, m := range pathWildcardRe.FindAllStringSubmatch(t.request.Pattern, -1) {
		key := m[1]
		if val := t.request.PathValue(key); val != "" {
			attrs = append(attrs, slog.String(key, val))
		}
	}

	return slog.GroupValue(attrs...)
}

// TrackElapsed is a helper to easily create the attribute
func TrackElapsed() any {
	return elapsedTracker{start: time.Now()}
}

// pathWildcardRe matches {key} and {key...}. Ignores the strict slash {$} token.
var pathWildcardRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+)(?:\.\.\.)?\}`)

func AttrsFromRequest(r *http.Request) []any {
	var attrs []any

	// 1. Extract OpenTelemetry Trace and Span IDs
	spanCtx := trace.SpanFromContext(r.Context()).SpanContext()
	if spanCtx.IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	attrs = append(
		attrs,
		slog.Any("", requestRouteAttrsTracker{request: r}),
		// 2. Add the elapsed time since the request started
		slog.Any("elapsed", TrackElapsed()),
	)

	return attrs
}
