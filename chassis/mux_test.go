package chassis_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lgosse/goforge/httpmiddlewares"

	"github.com/lgosse/goforge/chassis"
)

func TestNewServeMuxWithoutOptionsIsNaked(t *testing.T) {
	mux := chassis.NewServeMux()
	mux.HandleFunc("GET /tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Pattern != "GET /tasks/{id}" {
			t.Fatalf("expected request pattern %q, got %q", "GET /tasks/{id}", r.Pattern)
		}
		if r.PathValue("id") != "task-1" {
			t.Fatalf("expected path value %q, got %q", "task-1", r.PathValue("id"))
		}

		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestServeMuxHandleAppliesConfiguredMiddleware(t *testing.T) {
	mux := chassis.NewServeMux(chassis.WithAPIKey("secret"))
	mux.Handle("GET /private", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	unauthorizedRecorder := httptest.NewRecorder()
	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/private", nil)
	mux.ServeHTTP(unauthorizedRecorder, unauthorizedRequest)

	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, unauthorizedRecorder.Code)
	}

	authorizedRecorder := httptest.NewRecorder()
	authorizedRequest := httptest.NewRequest(http.MethodGet, "/private", nil)
	authorizedRequest.Header.Set("X-Api-Key", "secret")
	mux.ServeHTTP(authorizedRecorder, authorizedRequest)

	if authorizedRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, authorizedRecorder.Code)
	}
}

func TestServeMuxMiddlewarePatternExclusionRunsAfterRouting(t *testing.T) {
	mux := chassis.NewServeMux(
		chassis.WithAPIKey(
			"secret",
			httpmiddlewares.WithMuxPatternExclusion("GET /health"),
		),
	)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /private", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	healthRecorder := httptest.NewRecorder()
	mux.ServeHTTP(
		healthRecorder,
		httptest.NewRequest(http.MethodGet, "/health", nil),
	)
	if healthRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected excluded route status %d, got %d", http.StatusNoContent, healthRecorder.Code)
	}

	privateRecorder := httptest.NewRecorder()
	mux.ServeHTTP(
		privateRecorder,
		httptest.NewRequest(http.MethodGet, "/private", nil),
	)
	if privateRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected route status %d, got %d", http.StatusUnauthorized, privateRecorder.Code)
	}
}

func TestServeMuxMiddlewarePatternInclusion(t *testing.T) {
	mux := chassis.NewServeMux(
		chassis.WithAPIKey(
			"secret",
			httpmiddlewares.WithMuxPatternInclusion("GET /private"),
		),
	)
	mux.HandleFunc("GET /public", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /private", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	publicRecorder := httptest.NewRecorder()
	mux.ServeHTTP(
		publicRecorder,
		httptest.NewRequest(http.MethodGet, "/public", nil),
	)
	if publicRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected non-included route status %d, got %d", http.StatusNoContent, publicRecorder.Code)
	}

	privateRecorder := httptest.NewRecorder()
	mux.ServeHTTP(
		privateRecorder,
		httptest.NewRequest(http.MethodGet, "/private", nil),
	)
	if privateRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected included route status %d, got %d", http.StatusUnauthorized, privateRecorder.Code)
	}
}

func TestWithDefaultChassisUsesExplicitLoggerAndRecovers(t *testing.T) {
	for _, test := range []struct {
		name    string
		options func(*slog.Logger) []chassis.Option
	}{
		{
			name: "default before explicit logger",
			options: func(logger *slog.Logger) []chassis.Option {
				return []chassis.Option{
					chassis.WithDefaultChassis(),
					chassis.WithLogger(logger),
				}
			},
		},
		{
			name: "default after explicit logger",
			options: func(logger *slog.Logger) []chassis.Option {
				return []chassis.Option{
					chassis.WithLogger(logger),
					chassis.WithDefaultChassis(),
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
			mux := chassis.NewServeMux(test.options(logger)...)
			mux.HandleFunc("GET /panic/{id}", func(http.ResponseWriter, *http.Request) {
				panic("boom")
			})

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodGet, "/panic/test", nil),
			)

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
			}

			var entry map[string]any
			if err := json.NewDecoder(&logBuffer).Decode(&entry); err != nil {
				t.Fatalf("failed to decode recovery log: %v", err)
			}

			httpRequest, ok := entry["http_request"].(map[string]any)
			if !ok {
				t.Fatalf("expected http_request object, got %#v", entry["http_request"])
			}
			if httpRequest["route"] != "GET /panic/{id}" {
				t.Fatalf("expected route %q, got %#v", "GET /panic/{id}", httpRequest["route"])
			}
			if httpRequest["id"] != "test" {
				t.Fatalf("expected path value %q, got %#v", "test", httpRequest["id"])
			}
		})
	}
}

func TestServeMuxUsesCanonicalMiddlewareOrder(t *testing.T) {
	mux := chassis.NewServeMux(
		chassis.WithAPIKey("secret"),
		chassis.WithCORS(httpmiddlewares.CORSConfig{
			AllowedOrigins: []string{"https://app.example.com"},
		}, httpmiddlewares.WithMuxPatternInclusion("POST /private")),
	)
	mux.HandleFunc("POST /private", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	request := httptest.NewRequest(http.MethodOptions, "/private", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected CORS preflight status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf(
			"expected allowed origin %q, got %q",
			"https://app.example.com",
			recorder.Header().Get("Access-Control-Allow-Origin"),
		)
	}
}

func TestWithSharedCachingWithoutRedisPassesThrough(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	mux := chassis.NewServeMux(
		chassis.WithSharedCaching(logger, "api.example.com", nil),
	)
	mux.HandleFunc("GET /public", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/public", nil),
	)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}

	var entry map[string]any
	if err := json.NewDecoder(&logBuffer).Decode(&entry); err != nil {
		t.Fatalf("failed to decode shared caching log: %v", err)
	}
	if entry["msg"] != "missing redis client" {
		t.Fatalf("expected missing Redis log, got %#v", entry["msg"])
	}
}

func TestServeMuxHandlerReturnsRegisteredMatch(t *testing.T) {
	mux := chassis.NewServeMux()
	mux.HandleFunc("GET /tasks/{id}", func(http.ResponseWriter, *http.Request) {})

	handler, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil))

	if handler == nil {
		t.Fatal("expected a handler")
	}
	if pattern != "GET /tasks/{id}" {
		t.Fatalf("expected pattern %q, got %q", "GET /tasks/{id}", pattern)
	}
}

func TestServeMuxRejectsNilHandlerFunc(t *testing.T) {
	mux := chassis.NewServeMux(chassis.WithDefaultChassis())

	defer func() {
		if recover() == nil {
			t.Fatal("expected nil handler registration to panic")
		}
	}()

	mux.HandleFunc("GET /tasks", nil)
}
