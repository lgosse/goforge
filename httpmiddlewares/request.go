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

// LogValue is called by slog right when the log is being written.
func (e elapsedTracker) LogValue() slog.Value {
	return slog.DurationValue(time.Since(e.start))
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

	// 2. Extract path variables using regex
	matches := pathWildcardRe.FindAllStringSubmatch(r.Pattern, -1)
	for _, m := range matches {
		key := m[1]
		// 3. Fetch the value natively
		if val := r.PathValue(key); val != "" {
			attrs = append(attrs, slog.String(key, val))
		}
	}

	attrs = append(
		attrs,
		slog.String("route", r.Pattern),
		// 4. Add the elapsed time since the request started
		slog.Any("elapsed", TrackElapsed()),
	)

	return attrs
}
