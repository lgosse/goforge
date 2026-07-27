package httpclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAPIKeyTransportClonesRequest(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/tasks", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	var received *http.Request
	transport := &APIKeyTransport{
		APIKey: "secret",
		Next: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			received = req
			return testResponse(), nil
		}),
	}

	resp, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close()

	if received == request {
		t.Fatal("expected request to be cloned")
	}
	if received.Header.Get(defaultAPIKeyHeader) != "secret" {
		t.Fatalf("expected API key header, got %q", received.Header.Get(defaultAPIKeyHeader))
	}
	if request.Header.Get(defaultAPIKeyHeader) != "" {
		t.Fatalf("expected original request to remain unchanged, got %q", request.Header.Get(defaultAPIKeyHeader))
	}
}

func TestOAuthTransportDeduplicatesConcurrentRefreshes(t *testing.T) {
	var providerCalls atomic.Int32
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	transport := &OAuthTransport{
		Provider: func(context.Context) (OAuthToken, error) {
			if providerCalls.Add(1) == 1 {
				close(refreshStarted)
			}
			<-releaseRefresh
			return OAuthToken{
				AccessToken: "token",
				ExpiresAt:   time.Now().Add(time.Hour),
			}, nil
		},
		Next: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("expected Bearer token, got %q", req.Header.Get("Authorization"))
			}
			return testResponse(), nil
		}),
	}

	request, err := http.NewRequest(http.MethodGet, "https://example.com/tasks", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	const requestCount = 32
	var group sync.WaitGroup
	group.Add(requestCount)
	errorsCh := make(chan error, requestCount)
	for range requestCount {
		go func() {
			defer group.Done()

			resp, err := transport.RoundTrip(request)
			if resp != nil {
				_ = resp.Body.Close()
			}
			errorsCh <- err
		}()
	}

	<-refreshStarted
	close(releaseRefresh)
	group.Wait()
	close(errorsCh)

	for err := range errorsCh {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("expected one provider call, got %d", providerCalls.Load())
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatalf("expected original request to remain unchanged, got %q", request.Header.Get("Authorization"))
	}
}

func TestOAuthTransportWaiterCanBeCanceled(t *testing.T) {
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	transport := &OAuthTransport{
		Provider: func(context.Context) (OAuthToken, error) {
			close(refreshStarted)
			<-releaseRefresh
			return OAuthToken{AccessToken: "token"}, nil
		},
		Next: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return testResponse(), nil
		}),
	}

	firstRequest, err := http.NewRequest(http.MethodGet, "https://example.com/tasks", nil)
	if err != nil {
		t.Fatalf("failed to create first request: %v", err)
	}
	firstDone := make(chan error, 1)
	go func() {
		resp, err := transport.RoundTrip(firstRequest)
		if resp != nil {
			_ = resp.Body.Close()
		}
		firstDone <- err
	}()

	<-refreshStarted
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	secondRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/tasks", nil)
	if err != nil {
		t.Fatalf("failed to create second request: %v", err)
	}

	resp, err := transport.RoundTrip(secondRequest)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("expected context cancellation, got %v", err)
	}

	close(releaseRefresh)
	if err := <-firstDone; err != nil {
		t.Fatalf("expected first request to finish, got %v", err)
	}
}

func TestTelemetryTransportClonesRequest(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/tasks", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	var received *http.Request
	transport := &TelemetryTransport{
		Next: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			received = req
			return testResponse(), nil
		}),
	}

	resp, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer resp.Body.Close()

	if received == request {
		t.Fatal("expected request to be cloned")
	}
}

func TestRoundTrippersRejectNilRequests(t *testing.T) {
	for name, transport := range map[string]http.RoundTripper{
		"API key":   &APIKeyTransport{},
		"OAuth":     &OAuthTransport{},
		"telemetry": &TelemetryTransport{},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := transport.RoundTrip(nil)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func testResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
	}
}
