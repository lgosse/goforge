package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	otelhttp "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const defaultAPIKeyHeader = "X-API-Key" //nolint:gosec // This is a header name, not a credential.

// APIKeyTransport injects a static API key header into outbound requests.
type APIKeyTransport struct {
	Next   http.RoundTripper
	Header string
	APIKey string
}

// RoundTrip clones req, injects the configured API key, and delegates to Next.
func (t *APIKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("API key transport: nil request")
	}

	clonedReq := req.Clone(req.Context())
	if clonedReq.Header == nil {
		clonedReq.Header = make(http.Header)
	}

	header := t.Header
	if header == "" {
		header = defaultAPIKeyHeader
	}
	clonedReq.Header.Set(header, t.APIKey)

	return transportOrDefault(t.Next).RoundTrip(clonedReq)
}

// OAuthToken contains a Bearer token and its expiration time. A zero ExpiresAt
// means the token does not expire.
type OAuthToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

// TokenProvider fetches a Bearer token for an outbound request.
type TokenProvider func(context.Context) (OAuthToken, error)

// OAuthTransport injects cached Bearer authentication into outbound requests.
// It deduplicates concurrent refreshes and allows waiting requests to be
// canceled through their contexts.
type OAuthTransport struct {
	Next         http.RoundTripper
	Provider     TokenProvider
	ExpiryLeeway time.Duration

	mu          sync.RWMutex
	token       OAuthToken
	refreshing  bool
	refreshDone chan struct{}
	refreshErr  error
}

// RoundTrip clones req, obtains a valid token, injects it, and delegates to
// Next.
func (t *OAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("OAuth transport: nil request")
	}

	clonedReq := req.Clone(req.Context())
	if clonedReq.Header == nil {
		clonedReq.Header = make(http.Header)
	}

	token, err := t.accessToken(clonedReq.Context())
	if err != nil {
		return nil, fmt.Errorf("fetch OAuth token: %w", err)
	}
	clonedReq.Header.Set("Authorization", "Bearer "+token)

	return transportOrDefault(t.Next).RoundTrip(clonedReq)
}

func (t *OAuthTransport) accessToken(ctx context.Context) (string, error) {
	for {
		if token, ok := t.cachedToken(time.Now()); ok {
			return token, nil
		}

		t.mu.Lock()
		if t.tokenValid(time.Now()) {
			token := t.token.AccessToken
			t.mu.Unlock()
			return token, nil
		}
		if t.refreshing {
			refreshDone := t.refreshDone
			t.mu.Unlock()

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-refreshDone:
			}

			t.mu.RLock()
			refreshErr := t.refreshErr
			t.mu.RUnlock()
			if refreshErr != nil {
				return "", refreshErr
			}
			continue
		}
		if t.Provider == nil {
			t.mu.Unlock()
			return "", errors.New("missing token provider")
		}

		t.refreshing = true
		t.refreshDone = make(chan struct{})
		t.refreshErr = nil
		refreshDone := t.refreshDone
		provider := t.Provider
		t.mu.Unlock()

		token, err := provider(ctx)
		if err == nil && token.AccessToken == "" {
			err = errors.New("token provider returned an empty access token")
		}

		t.mu.Lock()
		if err == nil {
			t.token = token
		}
		t.refreshErr = err
		t.refreshing = false
		close(refreshDone)
		t.mu.Unlock()

		if err != nil {
			return "", err
		}

		return token.AccessToken, nil
	}
}

func (t *OAuthTransport) cachedToken(now time.Time) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.tokenValid(now) {
		return "", false
	}

	return t.token.AccessToken, true
}

func (t *OAuthTransport) tokenValid(now time.Time) bool {
	if t.token.AccessToken == "" {
		return false
	}
	if t.token.ExpiresAt.IsZero() {
		return true
	}

	return now.Add(t.ExpiryLeeway).Before(t.token.ExpiresAt)
}

// TelemetryTransport instruments outbound requests with OpenTelemetry.
type TelemetryTransport struct {
	Next    http.RoundTripper
	Options []otelhttp.Option

	once      sync.Once
	transport http.RoundTripper
}

// RoundTrip clones req and delegates to an OpenTelemetry transport.
func (t *TelemetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("telemetry transport: nil request")
	}

	clonedReq := req.Clone(req.Context())
	t.once.Do(func() {
		t.transport = otelhttp.NewTransport(
			transportOrDefault(t.Next),
			t.Options...,
		)
	})

	return transportOrDefault(t.transport).RoundTrip(clonedReq)
}
