# httpmiddlewares

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/httpmiddlewares.svg)](https://pkg.go.dev/github.com/lgosse/goforge/httpmiddlewares)

Package `httpmiddlewares` provides composable middleware for standard
`net/http` servers.

Every middleware uses the conventional
`func(http.Handler) http.Handler` shape, so it can be used with `http.ServeMux`,
`chassis`, or another compatible router.

## Install

```sh
go get github.com/lgosse/goforge/httpmiddlewares@latest
```

## Middleware

| Middleware | Purpose |
| --- | --- |
| `OpenTelemetryMiddleware` | Creates server spans and HTTP metrics |
| `LoggerMiddleware` | Adds request attributes and trace IDs to a scoped logger |
| `RecoverMiddleware` | Recovers panics and returns an internal error |
| `CORSMiddleware` | Applies CORS headers and handles preflight requests |
| `APIKeyMiddleware` | Validates the `X-Api-Key` request header |
| `SharedCachingMiddleware` | Shares cached responses through Redis |

## Example

Middleware can be wrapped manually:

```go
var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

handler = httpmiddlewares.RecoverMiddleware()(handler)
handler = httpmiddlewares.LoggerMiddleware(logger)(handler)
handler = httpmiddlewares.OpenTelemetryMiddleware()(handler)

mux := http.NewServeMux()
mux.Handle("GET /health", handler)
```

The outermost middleware is the last wrapper assigned.

## Route inclusion and exclusion

Each middleware accepts options that scope it to standard `http.ServeMux`
patterns:

```go
loggerMiddleware := httpmiddlewares.LoggerMiddleware(
	logger,
	httpmiddlewares.WithMuxPatternExclusion("GET /health"),
)

adminAuthentication := httpmiddlewares.APIKeyMiddleware(
	apiKey,
	httpmiddlewares.WithMuxPatternInclusion("GET /admin/{id}"),
)
```

Pattern scoping depends on `request.Pattern`, so the mux must resolve the route
before the middleware runs. `chassis` handles this automatically by wrapping
handlers when they are registered.

## CORS

```go
cors := httpmiddlewares.CORSMiddleware(httpmiddlewares.CORSConfig{
	AllowedOrigins:   []string{"https://*.example.com"},
	AllowedMethods:   []string{http.MethodGet, http.MethodPost},
	AllowedHeaders:   []string{"Authorization", "Content-Type"},
	ExposedHeaders:   []string{"X-Request-ID"},
	AllowCredentials: true,
	MaxAge:           10 * time.Minute,
})
```

The middleware handles valid preflight requests directly unless
`PassthroughPreflight` is enabled.

## Request logging

`LoggerMiddleware` stores the request-scoped logger in the request context.
Handlers retrieve it through the root module:

```go
logger := goforge.LoggerFromContext(r.Context())
logger.InfoContext(r.Context(), "request handled")
```

When OpenTelemetry middleware runs before the logger, request attributes
include `trace_id` and `span_id`.

The [`otel`](https://pkg.go.dev/github.com/lgosse/goforge/otel) module supplies
explicit trace and metric providers:

```go
handler := httpmiddlewares.OpenTelemetryMiddleware(
	telemetry.HTTPServerOptions()...,
)(applicationHandler)
```

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
