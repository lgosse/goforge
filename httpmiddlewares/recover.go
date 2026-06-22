package httpmiddlewares

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/lgosse/goforge"
)

const recoverMiddlewareName = "recover"

// RecoverMiddleware recovers panics, logs the failure through the request
// logger, and returns a standard goforge internal error when the response has
// not already started.
func RecoverMiddleware(opts ...middlewareOption) func(http.Handler) http.Handler {
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

			tracker := &recoverResponseWriter{ResponseWriter: w}
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				cause := recoveredPanicError(recovered)
				forgeErr := goforge.NewError(cause).
					WithHTTPStatus(http.StatusInternalServerError).
					WithCode("ERR_PANIC_RECOVERED").
					WithMessage("Internal server error")

				logger := goforge.LoggerFromContext(r.Context()).With(
					slog.String("middleware", recoverMiddlewareName),
				)

				logger.Error(
					"recovered panic",
					slog.Any("error", forgeErr),
					slog.Any("panic", recovered),
					slog.Bool("response_started", tracker.started()),
				)

				if tracker.started() {
					return
				}

				if err := goforge.RespondError(tracker, forgeErr); err != nil {
					logger.Warn("failed to respond with recovered panic error", slog.Any("error", err))
				}
			}()

			next.ServeHTTP(tracker, r)
		})
	}
}

type recoverResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *recoverResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}

	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *recoverResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(body)
}

func (w *recoverResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *recoverResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *recoverResponseWriter) started() bool {
	return w.wroteHeader
}

func recoveredPanicError(recovered any) error {
	switch value := recovered.(type) {
	case nil:
		return errors.New("panic recovered")
	case error:
		return value
	case string:
		return errors.New(value)
	default:
		return fmt.Errorf("%v", value)
	}
}
