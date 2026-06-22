package httpmiddlewares

import (
	"log/slog"
	"net/http"

	"github.com/lgosse/goforge"
)

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

			logger := logger.With(slog.Group(
				"http_request",
				AttrsFromRequest(r)...,
			))

			// Add the logger to the request context
			ctxWithLogger := goforge.WithLogger(r.Context(), logger)
			r = r.WithContext(ctxWithLogger)

			// Call the next handler in the chain
			next.ServeHTTP(w, r)
		}
	}
}
