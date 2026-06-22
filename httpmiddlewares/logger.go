package httpmiddlewares

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/lgosse/goforge"
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

func LoggerMiddleware(logger *slog.Logger, opts ...middlewareOption) func(http.HandlerFunc) http.HandlerFunc {
	var options middlewareOptions
	for _, opt := range opts {
		opt(&options)
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if options.shouldExclude(r) {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			var attrs []any

			// 1. Extract OpenTelemetry Trace and Span IDs
			spanCtx := trace.SpanFromContext(ctx).SpanContext()
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

			logger = logger.With(slog.Group(
				"http_request",
				attrs...,
			))

			// Add the logger to the request context
			ctxWithLogger := goforge.WithLogger(ctx, logger)
			r = r.WithContext(ctxWithLogger)

			// Call the next handler in the chain
			next.ServeHTTP(w, r)
		}
	}
}
