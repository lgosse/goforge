# chassis

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/chassis.svg)](https://pkg.go.dev/github.com/lgosse/goforge/chassis)

Package `chassis` builds middleware-aware HTTP servers on top of the standard
library's `http.ServeMux`.

A new chassis is deliberately lightweight: calling `NewServeMux()` without
options behaves like a nearly naked `http.ServeMux`. Functional options add
only the service concerns an application needs.

## Install

```sh
go get github.com/lgosse/goforge/chassis@latest
```

## Quick start

```go
telemetryConfig := forgeotel.DefaultConfig("orders-api", localDevelopment)
telemetryConfig.OTLP.Endpoint = "otel-collector.internal:4317" // Production only.
telemetry, err := forgeotel.New(context.Background(), telemetryConfig)
if err != nil {
	log.Fatal(err)
}
defer telemetry.Shutdown(context.Background())

mux := chassis.NewServeMux(
	chassis.WithDefaultChassis(),
	chassis.WithOpenTelemetry(telemetry.HTTPServerOptions()...),
	chassis.WithLogger(telemetry.Logger()),
	chassis.WithCORS(httpmiddlewares.CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost},
	}),
	chassis.WithAPIKey(os.Getenv("API_KEY")),
)

mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

server := &http.Server{
	Addr:         ":8080",
	Handler:      mux,
	ReadTimeout:  15 * time.Second,
	WriteTimeout: 15 * time.Second,
	IdleTimeout:  60 * time.Second,
}

log.Fatal(server.ListenAndServe())
```

`ServeMux` implements `http.Handler` and exposes familiar `Handle`,
`HandleFunc`, and `Handler` methods.

## Options

| Option | Behavior |
| --- | --- |
| `WithDefaultChassis` | Enables request logging and panic recovery |
| `WithOpenTelemetry` | Adds server tracing and metrics through `otelhttp` |
| `WithLogger` | Adds a request-scoped `slog.Logger` |
| `WithRecover` | Converts panics into HTTP 500 responses |
| `WithCORS` | Handles browser CORS requests and preflights |
| `WithAPIKey` | Requires an `X-Api-Key` header |
| `WithSharedCaching` | Adds Redis-backed shared response caching |
| `WithMiddleware` | Appends arbitrary `func(http.Handler) http.Handler` middleware |

The [`otel`](https://pkg.go.dev/github.com/lgosse/goforge/otel) module provides
explicit production and local-development providers for `WithOpenTelemetry`.
Calling the option without a provider uses OpenTelemetry's process-wide
provider, which is a no-op until the application configures it.

Explicit logger and recovery options override their default-chassis
counterparts regardless of option order.

## Middleware order

GoForge middleware is assembled in this order:

```text
OpenTelemetry → logger → recovery → CORS → API key → shared caching → custom
```

Custom middleware passed to `WithMiddleware` runs in the order provided.
Middleware is attached when a handler is registered, after the standard mux has
resolved its route pattern and path values.

## Route scoping

GoForge middleware can be included or excluded by standard-library mux pattern:

```go
mux := chassis.NewServeMux(
	chassis.WithLogger(
		logger,
		httpmiddlewares.WithMuxPatternExclusion("GET /health"),
	),
	chassis.WithAPIKey(
		os.Getenv("API_KEY"),
		httpmiddlewares.WithMuxPatternInclusion("GET /admin/{id}"),
	),
)
```

Use the exact pattern passed to `Handle` or `HandleFunc`.

## Shared caching

Shared caching requires a configured `github.com/redis/go-redis/v9` client and
a reachable Redis server:

```go
redisClient := redis.NewClient(&redis.Options{
	Addr: "localhost:6379",
})

mux := chassis.NewServeMux(
	chassis.WithSharedCaching(
		logger,
		"users",
		redisClient,
		httpmiddlewares.WithMuxPatternInclusion("GET /users/{id}"),
	),
)
```

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
