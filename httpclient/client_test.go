package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClientUsesOptimizedDefaults(t *testing.T) {
	client := NewClient()

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if client.Timeout != defaultClientTimeout {
		t.Fatalf("expected timeout %s, got %s", defaultClientTimeout, client.Timeout)
	}
	if transport.MaxIdleConns != defaultMaxIdleConns {
		t.Fatalf("expected %d idle connections, got %d", defaultMaxIdleConns, transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != defaultMaxIdleConnsPerHost {
		t.Fatalf(
			"expected %d idle connections per host, got %d",
			defaultMaxIdleConnsPerHost,
			transport.MaxIdleConnsPerHost,
		)
	}
	if transport.ResponseHeaderTimeout != defaultResponseHeaderTimeout {
		t.Fatalf(
			"expected response header timeout %s, got %s",
			defaultResponseHeaderTimeout,
			transport.ResponseHeaderTimeout,
		)
	}
}

func TestNewClientTransportOptionsWrapLikeOnion(t *testing.T) {
	client := NewClient(
		WithAPIKey("", "secret"),
		WithTelemetry(),
	)

	telemetry, ok := client.Transport.(*TelemetryTransport)
	if !ok {
		t.Fatalf("expected *TelemetryTransport, got %T", client.Transport)
	}
	if _, ok := telemetry.Next.(*APIKeyTransport); !ok {
		t.Fatalf("expected API key transport inside telemetry, got %T", telemetry.Next)
	}
}

func TestNewClientOptionsOverrideDefaults(t *testing.T) {
	timeout := 5 * time.Second
	client := NewClient(
		WithTransport(nil),
		WithTimeout(timeout),
	)

	if client.Transport != http.DefaultTransport {
		t.Fatalf("expected default transport, got %T", client.Transport)
	}
	if client.Timeout != timeout {
		t.Fatalf("expected timeout %s, got %s", timeout, client.Timeout)
	}
}

func TestWithTransportFallsBackForTypedNil(t *testing.T) {
	var transport *http.Transport
	client := NewClient(WithTransport(transport))

	if client.Transport != http.DefaultTransport {
		t.Fatalf("expected default transport, got %T", client.Transport)
	}
}
