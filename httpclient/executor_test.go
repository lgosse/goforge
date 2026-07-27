package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lgosse/goforge"
)

type callResponse struct {
	ID string `json:"id"`
}

func TestCallBuildsAndDecodesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("expected method %s, got %s", http.MethodPost, req.Method)
		}
		if req.URL.Path != "/api/tasks" {
			t.Errorf("expected path %q, got %q", "/api/tasks", req.URL.Path)
		}
		if req.URL.Query().Get("status") != "active" {
			t.Errorf("expected query parameter, got %q", req.URL.RawQuery)
		}
		if req.Header.Get("X-Request-ID") != "request-1" {
			t.Errorf("expected request header, got %q", req.Header.Get("X-Request-ID"))
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected JSON content type, got %q", req.Header.Get("Content-Type"))
		}

		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["name"] != "test" {
			t.Errorf("expected request body, got %#v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task-1"}`))
	}))
	defer server.Close()

	resp, err := Call[callResponse](
		context.Background(),
		server.Client(),
		http.MethodPost,
		server.URL+"/api",
		"tasks",
		map[string]string{"name": "test"},
		&RequestOpts{
			Headers: http.Header{"X-Request-ID": {"request-1"}},
			Query:   map[string][]string{"status": {"active"}},
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.ID != "task-1" {
		t.Fatalf("expected task ID %q, got %q", "task-1", resp.ID)
	}
}

func TestCallReturnsGoforgeErrorForHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"ERR_TASK_NOT_FOUND","message":"Task not found"}`))
	}))
	defer server.Close()

	resp, err := Call[callResponse](
		context.Background(),
		server.Client(),
		http.MethodGet,
		server.URL,
		"tasks/task-1",
		nil,
		nil,
	)

	if resp != nil {
		t.Fatalf("expected nil response, got %#v", resp)
	}
	var forgeErr *goforge.Error
	if !errors.As(err, &forgeErr) {
		t.Fatalf("expected *goforge.Error, got %T: %v", err, err)
	}
	if forgeErr.HTTPStatus != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, forgeErr.HTTPStatus)
	}
	if forgeErr.Code != "ERR_TASK_NOT_FOUND" {
		t.Fatalf("expected code %q, got %q", "ERR_TASK_NOT_FOUND", forgeErr.Code)
	}
	if forgeErr.Message != "Task not found" {
		t.Fatalf("expected message %q, got %q", "Task not found", forgeErr.Message)
	}
}

func TestCallReturnsGoforgeErrorForMalformedHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("not JSON"))
	}))
	defer server.Close()

	_, err := Call[callResponse](
		context.Background(),
		server.Client(),
		http.MethodGet,
		server.URL,
		"tasks",
		nil,
		nil,
	)

	var forgeErr *goforge.Error
	if !errors.As(err, &forgeErr) {
		t.Fatalf("expected *goforge.Error, got %T: %v", err, err)
	}
	if forgeErr.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, forgeErr.HTTPStatus)
	}
	if forgeErr.Code != defaultHTTPErrorCode {
		t.Fatalf("expected code %q, got %q", defaultHTTPErrorCode, forgeErr.Code)
	}
}

func TestCallAllowsEmptySuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	resp, err := Call[callResponse](
		context.Background(),
		server.Client(),
		http.MethodDelete,
		server.URL,
		"tasks/task-1",
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp == nil || resp.ID != "" {
		t.Fatalf("expected zero response, got %#v", resp)
	}
}

func TestCallAlwaysClosesResponseBody(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			body:       `{"id":"task-1"}`,
		},
		{
			name:       "HTTP failure",
			statusCode: http.StatusInternalServerError,
			body:       `{"code":"ERR_FAILURE","message":"failed"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			client := &http.Client{
				Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     http.StatusText(test.statusCode),
						StatusCode: test.statusCode,
						Header:     make(http.Header),
						Body:       body,
					}, nil
				}),
			}

			_, _ = Call[callResponse](
				context.Background(),
				client,
				http.MethodGet,
				"https://example.com",
				"tasks",
				nil,
				nil,
			)

			if !body.closed {
				t.Fatal("expected response body to be closed")
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}
