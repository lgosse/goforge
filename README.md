# goforge

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge.svg)](https://pkg.go.dev/github.com/lgosse/goforge)

goforge is a collection of small, composable Go modules for building HTTP
services with the standard library. It standardizes recurring service concerns
without introducing a web framework or hiding the underlying Go APIs.

The repository is organized around two primary entry points:

- **`chassis`** builds the inbound HTTP stack from `http.ServeMux` and opt-in
  middleware.
- **`httpclient`** builds the outbound HTTP stack from `http.Client`,
  composable transports, and a typed JSON executor.

The root module contains the contracts and primitives shared by those modules:
structured errors, JSON responses, context-scoped loggers, and endpoint
registration interfaces.

## Modules

Each directory containing a `go.mod` is an independently versioned Go module.
Applications only need to depend on the pieces they use.

| Module | Purpose | Status |
| --- | --- | --- |
| [`goforge`](https://pkg.go.dev/github.com/lgosse/goforge) | Shared errors, responses, contexts, and contracts | Available |
| [`chassis`](https://pkg.go.dev/github.com/lgosse/goforge/chassis) | Middleware-aware `http.ServeMux` | Available |
| [`httpmiddlewares`](https://pkg.go.dev/github.com/lgosse/goforge/httpmiddlewares) | Standard `net/http` middleware | Available |
| [`httpclient`](https://pkg.go.dev/github.com/lgosse/goforge/httpclient) | Configured HTTP clients and typed JSON calls | Available |
| [`forgemongo`](https://pkg.go.dev/github.com/lgosse/goforge/forgemongo) | Typed MongoDB stores and mocks | Available |
| [`forgesentry`](https://pkg.go.dev/github.com/lgosse/goforge/forgesentry) | `slog` integration for Sentry | Available |
| [`linters`](./linters) | Shared lint rules | Scaffold |

## Install

Install each module independently:

```sh
go get github.com/lgosse/goforge@latest
go get github.com/lgosse/goforge/chassis@latest
go get github.com/lgosse/goforge/httpclient@latest
```

## Building an HTTP service

`chassis.NewServeMux` behaves like a plain `http.ServeMux` when called without
options. Services can opt into the standard GoForge stack or select individual
middleware:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

mux := chassis.NewServeMux(
	chassis.WithDefaultChassis(),
	chassis.WithOpenTelemetry(),
	chassis.WithLogger(logger),
	chassis.WithCORS(httpmiddlewares.CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	}),
)

mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
	_ = goforge.RespondJSON(w, map[string]bool{"healthy": true}, http.StatusOK)
})

server := &http.Server{
	Addr:    ":8080",
	Handler: mux,
}

log.Fatal(server.ListenAndServe())
```

See the [`chassis` README](./chassis) for middleware ordering, route scoping,
and a fuller server example.

## Calling another service

`httpclient.NewClient` configures a high-throughput standard HTTP client.
Transport options wrap one another, allowing authentication and telemetry to
be composed:

```go
client := httpclient.NewClient(
	httpclient.WithTimeout(10*time.Second),
	httpclient.WithAPIKey("X-API-Key", os.Getenv("SERVICE_API_KEY")),
	httpclient.WithTelemetry(),
)

user, err := httpclient.Call[User](
	ctx,
	client,
	http.MethodGet,
	"https://users.internal",
	"/v1/users/user-1",
	nil,
	nil,
)
if err != nil {
	return err
}

fmt.Println(user.ID)
```

See the [`httpclient` README](./httpclient) for OAuth, request options, error
mapping, and transport composition.

## Versioning

The root module uses repository tags such as `v0.3.0`. Nested modules use tags
prefixed by their directory:

```text
chassis/v0.3.0
httpclient/v0.3.0
forgemongo/v0.3.0
```

Releasing one module does not require releasing every module.

## Development

Because this is a multi-module repository, root tests do not traverse nested
modules. Run checks from the module being changed:

```sh
cd httpclient
go test -race ./...
go vet ./...
```

## License

GoForge is available under the [MIT License](./LICENCE.txt).
