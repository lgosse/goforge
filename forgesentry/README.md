# forgesentry

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/forgesentry.svg)](https://pkg.go.dev/github.com/lgosse/goforge/forgesentry)

Package `forgesentry` connects Go's structured `log/slog` records to Sentry
while preserving the application's existing log handler.

Records below error level are only passed to the wrapped handler. Error-level
records are also reported to Sentry with their structured attributes.

## Install

```sh
go get github.com/lgosse/goforge/forgesentry@latest
```

## Usage

Initialize Sentry using its normal SDK configuration, then wrap the
application's handler:

```go
initErr := sentry.Init(sentry.ClientOptions{
	Dsn:              os.Getenv("SENTRY_DSN"),
	EnableTracing:    true,
	TracesSampleRate: 0.1,
})
if initErr != nil {
	log.Fatal(initErr)
}
defer sentry.Flush(2 * time.Second)

handler := slog.NewJSONHandler(os.Stdout, nil)
logger := forgesentry.NewLogger(handler)

logger.Info("service started", slog.String("version", version))
requestErr := errors.New("upstream request timed out")
logger.Error(
	"request failed",
	slog.String("request_id", requestID),
	slog.Any("error", requestErr),
)
```

`NewSentryHandler` is available when an application wants to assemble its own
`slog.Logger`:

```go
logger := slog.New(
	forgesentry.NewSentryHandler(
		slog.NewJSONHandler(os.Stdout, nil),
	),
)
```

## Error and attribute handling

- An attribute named `error` is used as the captured Sentry exception.
- Other structured attributes are added to the Sentry event context and
  extras.
- `With` attributes and nested `slog` groups are preserved.
- A Sentry hub attached to the logging context is preferred; otherwise the
  current global hub is cloned.
- Delivery to the wrapped handler is attempted before the Sentry event is
  captured.

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
