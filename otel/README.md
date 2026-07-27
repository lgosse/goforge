# otel

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/otel.svg)](https://pkg.go.dev/github.com/lgosse/goforge/otel)

Package `otel` provides one explicit, production-ready setup for OpenTelemetry
traces, metrics, logs, resources, propagation, and Go runtime metrics.

It owns the complete telemetry lifecycle and integrates directly with GoForge's
`chassis` and `httpclient` modules. It does not inspect `ENVIRONMENT`, `OTEL_*`,
or other environment variables.

## Install

```sh
go get github.com/lgosse/goforge/otel@latest
```

## Local development

Local mode needs no collector. It samples every trace, uses synchronous
processors, and writes readable text logs while still creating valid trace and
span identifiers:

```go
config := forgeotel.DefaultConfig("orders-api", true)

telemetry, err := forgeotel.New(context.Background(), config)
if err != nil {
	log.Fatal(err)
}
defer telemetry.Shutdown(context.Background())

logger := telemetry.Logger()
```

Always pass request or operation contexts to `InfoContext`, `ErrorContext`, and
the other context-aware slog methods. The handler adds `trace_id` and `span_id`
to console logs, while the OpenTelemetry log record retains native trace
correlation.

## Production

Production defaults enable secure OTLP/gRPC export for traces, metrics, and
logs, batch processing, JSON console logs, bounded queues and attributes,
parent-based 10% trace sampling, and Go runtime metrics:

```go
config := forgeotel.DefaultConfig("orders-api", false)
config.ServiceVersion = version
config.DeploymentEnvironment = "production"
config.OTLP.Endpoint = "otel-collector.internal:4317"
config.OTLP.Headers = map[string]string{
	"authorization": "Bearer " + collectorToken,
}

telemetry, err := forgeotel.New(context.Background(), config)
if err != nil {
	log.Fatal(err)
}
defer telemetry.Shutdown(context.Background())
```

Use `ProtocolHTTP` for OTLP/HTTP, or configure `OTLP.TLSConfig`,
`OTLP.Insecure`, sampling, batching, intervals, limits, custom exporters, and
custom metric readers directly on `Config`.

## GoForge HTTP integration

The runtime supplies explicit `otelhttp` options, so no global provider is
needed:

```go
mux := chassis.NewServeMux(
	chassis.WithDefaultChassis(),
	chassis.WithOpenTelemetry(telemetry.HTTPServerOptions()...),
	chassis.WithLogger(telemetry.Logger()),
)

client := httpclient.NewClient(
	httpclient.WithTelemetry(telemetry.HTTPClientOptions()...),
)
```

`InstallGlobals` is available for libraries that only consult process-wide
OpenTelemetry state. Prefer the explicit options above in application code.

## Shutdown

Call `Shutdown` with a bounded context during graceful process termination. It
flushes logs, metrics, and traces and is safe to call repeatedly:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := telemetry.Shutdown(shutdownCtx); err != nil {
	logger.Error("telemetry shutdown failed", "error", err)
}
```

## Stability

OpenTelemetry traces and metrics are stable upstream. The OpenTelemetry Go logs
SDK and slog bridge remain beta; GoForge exposes them behind the same runtime
lifecycle while following their versioned APIs. Profiles are not exposed
because the upstream Go SDK does not yet provide a profiles signal API.

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
