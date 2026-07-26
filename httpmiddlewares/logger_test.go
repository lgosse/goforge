package httpmiddlewares

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lgosse/goforge"
)

func TestLoggerMiddlewareUsesServeMuxPatternForRoute(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))

	mux := http.NewServeMux()
	mux.Handle("GET /tasks/{id}", LoggerMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goforge.LoggerFromContext(r.Context()).Info("success")
		w.WriteHeader(http.StatusOK)
	})))

	request := httptest.NewRequest(http.MethodGet, "/tasks/test", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var entry map[string]any
	if err := json.NewDecoder(&buffer).Decode(&entry); err != nil {
		t.Fatalf("failed to decode log entry: %v", err)
	}

	httpRequest, ok := entry["http_request"].(map[string]any)
	if !ok {
		t.Fatalf("expected http_request object, got %#v", entry["http_request"])
	}

	if got := httpRequest["route"]; got != "GET /tasks/{id}" {
		t.Fatalf("expected route %q, got %#v", "GET /tasks/{id}", got)
	}

	if got := httpRequest["id"]; got != "test" {
		t.Fatalf("expected path variable %q, got %#v", "test", got)
	}
}
