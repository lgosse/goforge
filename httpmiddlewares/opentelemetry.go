package httpmiddlewares

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultOpenTelemetryOperation = "HTTP server"

// OpenTelemetryMiddleware instruments HTTP requests with OpenTelemetry spans
// and metrics. Options are forwarded directly to [otelhttp.NewMiddleware];
// the otel module supplies explicit production-ready provider options.
func OpenTelemetryMiddleware(opts ...otelhttp.Option) func(http.Handler) http.Handler {
	return otelhttp.NewMiddleware(defaultOpenTelemetryOperation, opts...)
}
