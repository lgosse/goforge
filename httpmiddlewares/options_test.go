package httpmiddlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lgosse/goforge/httpmiddlewares"
)

func TestMiddlewareOptionsCanBeForwarded(t *testing.T) {
	options := []httpmiddlewares.MiddlewareOption{
		httpmiddlewares.WithMuxPatternExclusion("GET /health"),
	}
	handler := httpmiddlewares.APIKeyMiddleware("secret", options...)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Pattern = "GET /health"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}
