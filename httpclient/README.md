# httpclient

[![Go Reference](https://pkg.go.dev/badge/github.com/lgosse/goforge/httpclient.svg)](https://pkg.go.dev/github.com/lgosse/goforge/httpclient)

Package `httpclient` provides configured standard HTTP clients, composable
outbound transports, and a generic JSON request executor.

It is intended to be the networking foundation beneath small, typed
service-specific SDKs rather than a replacement for `net/http`.

## Install

```sh
go get github.com/lgosse/goforge/httpclient@latest
```

## Creating a client

```go
client := httpclient.NewClient(
	httpclient.WithTimeout(10*time.Second),
	httpclient.WithAPIKey("X-API-Key", os.Getenv("SERVICE_API_KEY")),
	httpclient.WithTelemetry(telemetry.HTTPClientOptions()...),
)
```

The default client uses a transport configured for high-throughput service
communication, including connection pooling, HTTP/2, and bounded dial,
handshake, response-header, and idle timeouts.

Available options are:

| Option | Behavior |
| --- | --- |
| `WithTransport` | Replaces the base `http.RoundTripper` |
| `WithTimeout` | Sets the timeout for a complete request |
| `WithAPIKey` | Adds a static authentication header |
| `WithOAuth` | Adds cached Bearer authentication |
| `WithTelemetry` | Instruments outbound calls through OpenTelemetry |

The [`otel`](https://pkg.go.dev/github.com/lgosse/goforge/otel) module constructs
the explicit providers used here:

```go
config := forgeotel.DefaultConfig("orders-api", localDevelopment)
config.OTLP.Endpoint = "otel-collector.internal:4317" // Production only.
telemetry, err := forgeotel.New(context.Background(), config)
if err != nil {
	return err
}
defer telemetry.Shutdown(context.Background())
```

Transport options wrap the transport configured by earlier options. In this
example, telemetry is outermost:

```go
client := httpclient.NewClient(
	httpclient.WithTransport(http.DefaultTransport),
	httpclient.WithAPIKey("X-API-Key", apiKey),
	httpclient.WithOAuth(tokenProvider, 30*time.Second),
	httpclient.WithTelemetry(telemetry.HTTPClientOptions()...),
)
```

Every authentication transport clones the request before modifying it, so the
middleware does not mutate caller-owned headers or URLs.

## OAuth

An OAuth token provider returns the token and its expiration time:

```go
tokenProvider := func(ctx context.Context) (httpclient.OAuthToken, error) {
	token, expiresAt, err := identityClient.FetchToken(ctx)
	if err != nil {
		return httpclient.OAuthToken{}, err
	}

	return httpclient.OAuthToken{
		AccessToken: token,
		ExpiresAt:   expiresAt,
	}, nil
}

client := httpclient.NewClient(
	httpclient.WithOAuth(tokenProvider, 30*time.Second),
)
```

Tokens are cached until their expiration window. Concurrent refreshes are
deduplicated, and callers waiting for a refresh can still cancel their context.

## Typed JSON calls

`Call` joins the base URL and endpoint, applies request options, handles JSON,
and decodes a typed response:

```go
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

user, err := httpclient.Call[User](
	ctx,
	client,
	http.MethodGet,
	"https://users.internal",
	"/v1/users/user-1",
	nil,
	&httpclient.RequestOpts{
		Headers: http.Header{
			"X-Request-ID": []string{requestID},
		},
		Query: url.Values{"expand": {"roles"}},
	},
)
```

For HTTP responses with a status code of 400 or greater, `Call` returns a
`*goforge.Error`. Transport, request-construction, and JSON errors remain
ordinary Go errors and can be wrapped by the calling SDK.

## License

This module is available under the repository's
[MIT License](../LICENCE.txt).
