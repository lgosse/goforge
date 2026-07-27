package httpclient

import (
	"net"
	"net/http"
	"reflect"
	"time"

	otelhttp "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	defaultClientTimeout         = 30 * time.Second
	defaultDialTimeout           = 5 * time.Second
	defaultKeepAlive             = 30 * time.Second
	defaultIdleConnTimeout       = 90 * time.Second
	defaultTLSHandshakeTimeout   = 5 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second
	defaultExpectContinueTimeout = time.Second
	defaultMaxIdleConns          = 256
	defaultMaxIdleConnsPerHost   = 64
)

// ClientOption configures an HTTP client. Transport options wrap the transport
// configured by preceding options, so the last transport option is outermost.
type ClientOption func(*http.Client)

// NewClient returns an HTTP client configured for high-throughput service
// communication.
func NewClient(opts ...ClientOption) *http.Client {
	client := &http.Client{
		Transport: newDefaultTransport(),
		Timeout:   defaultClientTimeout,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}

	return client
}

// WithTransport replaces the current client transport. A nil transport uses
// [http.DefaultTransport].
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(client *http.Client) {
		client.Transport = transportOrDefault(transport)
	}
}

// WithTimeout sets the maximum duration of a complete request.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(client *http.Client) {
		client.Timeout = timeout
	}
}

// WithAPIKey wraps the current transport with static API key authentication.
func WithAPIKey(header, apiKey string) ClientOption {
	return func(client *http.Client) {
		client.Transport = &APIKeyTransport{
			Next:   transportOrDefault(client.Transport),
			Header: header,
			APIKey: apiKey,
		}
	}
}

// WithOAuth wraps the current transport with cached Bearer authentication.
func WithOAuth(provider TokenProvider, expiryLeeway time.Duration) ClientOption {
	return func(client *http.Client) {
		client.Transport = &OAuthTransport{
			Next:         transportOrDefault(client.Transport),
			Provider:     provider,
			ExpiryLeeway: expiryLeeway,
		}
	}
}

// WithTelemetry wraps the current transport with OpenTelemetry
// instrumentation.
func WithTelemetry(opts ...otelhttp.Option) ClientOption {
	return func(client *http.Client) {
		client.Transport = &TelemetryTransport{
			Next:    transportOrDefault(client.Transport),
			Options: append([]otelhttp.Option(nil), opts...),
		}
	}
}

func newDefaultTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          defaultMaxIdleConns,
		MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultResponseHeaderTimeout,
		ExpectContinueTimeout: defaultExpectContinueTimeout,
	}
}

func transportOrDefault(transport http.RoundTripper) http.RoundTripper {
	if transport == nil {
		return http.DefaultTransport
	}
	value := reflect.ValueOf(transport)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return http.DefaultTransport
		}
	}

	return transport
}
