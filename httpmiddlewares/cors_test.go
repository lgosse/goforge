package httpmiddlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCORSMiddlewareActualRequests(t *testing.T) {
	for _, d := range []struct {
		name           string
		config         CORSConfig
		origin         string
		expectedOrigin string
		expectedVary   []string
		expectedCreds  string
		expectedExpose string
		expectCORS     bool
		expectNext     bool
	}{
		{
			name: "allows exact origin",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
				ExposedHeaders: []string{"X-Request-Id"},
			},
			origin:         "https://app.example.com",
			expectedOrigin: "https://app.example.com",
			expectedVary:   []string{headerOrigin},
			expectedExpose: "X-Request-Id",
			expectCORS:     true,
			expectNext:     true,
		},
		{
			name: "allows wildcard origin without credentials",
			config: CORSConfig{
				AllowedOrigins: []string{"*"},
			},
			origin:         "https://app.example.com",
			expectedOrigin: "*",
			expectCORS:     true,
			expectNext:     true,
		},
		{
			name: "echoes wildcard origin when credentials are allowed",
			config: CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowCredentials: true,
			},
			origin:         "https://app.example.com",
			expectedOrigin: "https://app.example.com",
			expectedVary:   []string{headerOrigin},
			expectedCreds:  "true",
			expectCORS:     true,
			expectNext:     true,
		},
		{
			name: "allows pattern origin",
			config: CORSConfig{
				AllowedOrigins: []string{"https://*.example.com"},
			},
			origin:         "https://admin.example.com",
			expectedOrigin: "https://admin.example.com",
			expectedVary:   []string{headerOrigin},
			expectCORS:     true,
			expectNext:     true,
		},
		{
			name: "allows dynamic origin",
			config: CORSConfig{
				AllowOriginFunc: func(origin string) bool {
					return origin == "https://tenant.example.com"
				},
			},
			origin:         "https://tenant.example.com",
			expectedOrigin: "https://tenant.example.com",
			expectedVary:   []string{headerOrigin},
			expectCORS:     true,
			expectNext:     true,
		},
		{
			name: "passes through disallowed actual origin without cors headers",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
			},
			origin:     "https://evil.example.com",
			expectNext: true,
		},
		{
			name: "passes through requests without origin",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
			},
			expectNext: true,
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			nextCalled := false
			handler := CORSMiddleware(d.config)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.Header().Set("X-Handler", "called")
				w.WriteHeader(http.StatusAccepted)
			}))

			request := httptest.NewRequest(http.MethodGet, "http://api.example.com/test", nil)
			if d.origin != "" {
				request.Header.Set(headerOrigin, d.origin)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			assert.Equal(t, d.expectNext, nextCalled)
			assert.Equal(t, http.StatusAccepted, recorder.Code)
			if d.expectCORS {
				assert.Equal(t, d.expectedOrigin, recorder.Header().Get(headerAccessControlAllowOrigin))
				assert.Equal(t, d.expectedCreds, recorder.Header().Get(headerAccessControlAllowCredentials))
				assert.Equal(t, d.expectedExpose, recorder.Header().Get(headerAccessControlExposeHeaders))
				assertVaryContains(t, recorder.Header(), d.expectedVary...)
			} else {
				assert.Empty(t, recorder.Header().Get(headerAccessControlAllowOrigin))
				assert.Empty(t, recorder.Header().Values(headerVary))
			}
		})
	}
}

func TestCORSMiddlewarePreflightRequests(t *testing.T) {
	for _, d := range []struct {
		name               string
		config             CORSConfig
		origin             string
		requestMethod      string
		requestHeaders     string
		privateNetwork     string
		expectedStatusCode int
		expectedOrigin     string
		expectedHeaders    string
		expectedMethods    string
		expectedMaxAge     string
		expectedPrivate    string
		expectNext         bool
		expectCORS         bool
	}{
		{
			name: "allows configured preflight",
			config: CORSConfig{
				AllowedOrigins:      []string{"https://app.example.com"},
				AllowedMethods:      []string{http.MethodGet, http.MethodPost},
				AllowedHeaders:      []string{"Authorization", "X-Request-Id"},
				MaxAge:              10 * time.Minute,
				AllowPrivateNetwork: true,
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodPost,
			requestHeaders:     "authorization, x-request-id",
			privateNetwork:     "true",
			expectedStatusCode: http.StatusNoContent,
			expectedOrigin:     "https://app.example.com",
			expectedHeaders:    "Authorization, X-Request-Id",
			expectedMethods:    "GET, POST",
			expectedMaxAge:     "600",
			expectedPrivate:    "true",
			expectCORS:         true,
		},
		{
			name: "reflects requested headers when allowed headers are empty",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodPatch,
			requestHeaders:     "x-custom, content-type",
			expectedStatusCode: http.StatusNoContent,
			expectedOrigin:     "https://app.example.com",
			expectedHeaders:    "X-Custom, Content-Type",
			expectedMethods:    "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS",
			expectCORS:         true,
		},
		{
			name: "rejects disallowed origin",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
			},
			origin:             "https://evil.example.com",
			requestMethod:      http.MethodGet,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name: "rejects disallowed method",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
				AllowedMethods: []string{http.MethodGet},
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodDelete,
			expectedStatusCode: http.StatusForbidden,
			expectedOrigin:     "https://app.example.com",
			expectCORS:         true,
		},
		{
			name: "rejects disallowed header",
			config: CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
				AllowedHeaders: []string{"Authorization"},
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodGet,
			requestHeaders:     "x-custom",
			expectedStatusCode: http.StatusForbidden,
			expectedOrigin:     "https://app.example.com",
			expectCORS:         true,
		},
		{
			name: "passes valid preflight through when configured",
			config: CORSConfig{
				AllowedOrigins:       []string{"https://app.example.com"},
				AllowedMethods:       []string{http.MethodGet},
				PassthroughPreflight: true,
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodGet,
			expectedStatusCode: http.StatusAccepted,
			expectedOrigin:     "https://app.example.com",
			expectedMethods:    "GET",
			expectNext:         true,
			expectCORS:         true,
		},
		{
			name: "uses custom preflight status code",
			config: CORSConfig{
				AllowedOrigins:      []string{"https://app.example.com"},
				PreflightStatusCode: http.StatusOK,
			},
			origin:             "https://app.example.com",
			requestMethod:      http.MethodGet,
			expectedStatusCode: http.StatusOK,
			expectedOrigin:     "https://app.example.com",
			expectedMethods:    "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS",
			expectCORS:         true,
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			nextCalled := false
			handler := CORSMiddleware(d.config)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusAccepted)
			}))

			request := httptest.NewRequest(http.MethodOptions, "http://api.example.com/test", nil)
			request.Header.Set(headerOrigin, d.origin)
			request.Header.Set(headerAccessControlRequestMethod, d.requestMethod)
			if d.requestHeaders != "" {
				request.Header.Set(headerAccessControlRequestHeaders, d.requestHeaders)
			}
			if d.privateNetwork != "" {
				request.Header.Set(headerAccessControlRequestPrivateNetwork, d.privateNetwork)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			assert.Equal(t, d.expectNext, nextCalled)
			assert.Equal(t, d.expectedStatusCode, recorder.Code)
			if d.expectCORS {
				assert.Equal(t, d.expectedOrigin, recorder.Header().Get(headerAccessControlAllowOrigin))
				assert.Equal(t, d.expectedMethods, recorder.Header().Get(headerAccessControlAllowMethods))
				assert.Equal(t, d.expectedHeaders, recorder.Header().Get(headerAccessControlAllowHeaders))
				assert.Equal(t, d.expectedMaxAge, recorder.Header().Get(headerAccessControlMaxAge))
				assert.Equal(t, d.expectedPrivate, recorder.Header().Get(headerAccessControlAllowPrivateNetwork))
				assertVaryContains(t, recorder.Header(), headerOrigin, headerAccessControlRequestMethod, headerAccessControlRequestHeaders)
			} else {
				assert.Empty(t, recorder.Header().Get(headerAccessControlAllowOrigin))
			}
		})
	}
}

func TestCORSMiddlewareRespectsMuxPatternOptions(t *testing.T) {
	nextCalled := false
	handler := CORSMiddleware(
		CORSConfig{AllowedOrigins: []string{"https://app.example.com"}},
		WithMuxPatternExclusion("GET /health"),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://api.example.com/health", nil)
	request.Pattern = "GET /health"
	request.Header.Set(headerOrigin, "https://app.example.com")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	assert.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Header().Get(headerAccessControlAllowOrigin))
}

func assertVaryContains(t *testing.T, header http.Header, expectedValues ...string) {
	t.Helper()

	values := header.Values(headerVary)
	for _, expectedValue := range expectedValues {
		assert.Contains(t, values, expectedValue)
	}
}
