package httpmiddlewares

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/lgosse/goforge"
)

// APIKeyMiddleware checks if the caller if authorized to contact the service.
func APIKeyMiddleware(apikey string, opts ...middlewareOption) func(http.Handler) http.Handler {
	var options middlewareOptions
	for _, opt := range opts {
		opt(&options)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if options.shouldExclude(r) {
				next.ServeHTTP(w, r)
				return
			}

			logger := goforge.LoggerFromContext(r.Context())

			if r.Header.Get("X-Api-Key") != apikey {
				if err := goforge.RespondError(w,
					goforge.NewError(errors.New("invalid API key")).
						WithHTTPStatus(http.StatusUnauthorized).
						WithCode("ERR_INVALID_API_KEY"),
				); err != nil {
					logger.Warn("failed to respond with error", slog.Any("error", err))
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
