# otel/mongo

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/otel/mongo.svg)](https://pkg.go.dev/github.com/lgosse/goforge/otel/mongo)

Package `otelmongo` connects MongoDB Go Driver v2 command monitoring to a
GoForge OpenTelemetry runtime.

It instruments MongoDB command spans and the standard
`db.client.operation.duration` metric. It does not create, connect, ping, or
disconnect a MongoDB client.

## Install

```sh
go get github.com/lgosse/goforge/otel/mongo@latest
```

## Usage

```go
telemetryConfig := forgeotel.DefaultConfig("users-api", localDevelopment)
telemetryConfig.OTLP.Endpoint = "otel-collector.internal:4317" // Production only.

telemetry, err := forgeotel.New(ctx, telemetryConfig)
if err != nil {
	return err
}
defer telemetry.Shutdown(context.Background())

monitor, err := forgeotelmongo.NewMonitor(telemetry)
if err != nil {
	return err
}

clientOptions := mongooptions.Client().
	ApplyURI(mongoURI).
	SetMonitor(monitor)

client, err := mongo.Connect(clientOptions)
if err != nil {
	return err
}
defer client.Disconnect(context.Background())
```

The monitor always uses the runtime's explicit tracer and meter providers.
Installing global OpenTelemetry providers is not required. Local and
production sampling and export behavior therefore follow the existing runtime
configuration automatically.

## Customization and command privacy

Options from the upstream OpenTelemetry MongoDB instrumentation are forwarded
without being reimplemented:

```go
monitor, err := forgeotelmongo.NewMonitor(
	telemetry,
	upstreamotelmongo.WithSpanNameFormatter(func(
		event *mongoevent.CommandStartedEvent,
	) string {
		return "mongodb." + event.CommandName
	}),
)
```

Full MongoDB command attributes are disabled by default. Enabling them with
`upstreamotelmongo.WithCommandAttributeDisabled(false)` may record query
values, personal data, secrets, and high-cardinality attributes. Keep the
default unless the application's data policy explicitly permits collection.

MongoDB supports one command monitor per client. Calling `SetMonitor` replaces
any monitor already configured on those client options.

## Dependency policy

The upstream MongoDB v2 instrumentation does not currently publish tagged
releases. This module pins an exact upstream commit aligned with the
OpenTelemetry v1.44 dependencies used by `goforge/otel`; upgrades should be
reviewed rather than following a moving pseudo-version.

## License

This module is available under the repository's
[MIT License](../../LICENCE.txt).
