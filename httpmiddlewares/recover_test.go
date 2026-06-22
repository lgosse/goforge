package httpmiddlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lgosse/goforge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoverMiddlewareRecoversAndResponds(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	handler := RecoverMiddleware()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	request = request.WithContext(goforge.WithLogger(request.Context(), logger))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"code":"ERR_PANIC_RECOVERED","message":"Internal server error"}`, recorder.Body.String())

	entry := decodeRecoverLog(t, &logBuffer)
	assert.Equal(t, "recovered panic", entry["msg"])
	assert.Equal(t, recoverMiddlewareName, entry["middleware"])
	assert.Equal(t, "boom", entry["panic"])
	assert.Equal(t, false, entry["response_started"])
	assert.NotContains(t, entry, "stack_trace")
	assert.Contains(t, entry["error"], "ERR_PANIC_RECOVERED")
}

func TestRecoverMiddlewareLogsErrorPanic(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	panicErr := errors.New("database is on fire")

	handler := RecoverMiddleware()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(panicErr)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	request = request.WithContext(goforge.WithLogger(request.Context(), logger))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	entry := decodeRecoverLog(t, &logBuffer)
	assert.Contains(t, entry["error"], "database is on fire")
}

func TestRecoverMiddlewareDoesNotWriteAfterResponseStarted(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	handler := RecoverMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("started"))
		panic("boom")
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	request = request.WithContext(goforge.WithLogger(request.Context(), logger))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusAccepted, recorder.Code)
	assert.Equal(t, "started", recorder.Body.String())

	entry := decodeRecoverLog(t, &logBuffer)
	assert.Equal(t, true, entry["response_started"])
}

func TestRecoverMiddlewareRespectsMuxPatternOptions(t *testing.T) {
	handler := RecoverMiddleware(WithMuxPatternExclusion("GET /panic"))(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/panic", nil)
	request.Pattern = "GET /panic"
	recorder := httptest.NewRecorder()

	assert.Panics(t, func() {
		handler.ServeHTTP(recorder, request)
	})
}

func TestRecoverMiddlewareDoesNotRecoverErrAbortHandler(t *testing.T) {
	handler := RecoverMiddleware()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	recorder := httptest.NewRecorder()

	recovered := recoverPanic(func() {
		handler.ServeHTTP(recorder, request)
	})
	assert.Same(t, http.ErrAbortHandler, recovered)
}

func TestRecoverResponseWriterSupportsResponseController(t *testing.T) {
	base := httptest.NewRecorder()
	tracker := &recoverResponseWriter{ResponseWriter: base}

	err := http.NewResponseController(tracker).Flush()

	assert.NoError(t, err)
	assert.True(t, tracker.started())
}

func TestRecoveredPanicError(t *testing.T) {
	assert.EqualError(t, recoveredPanicError(nil), "panic recovered")
	assert.EqualError(t, recoveredPanicError("boom"), "boom")
	assert.EqualError(t, recoveredPanicError(42), "42")

	err := errors.New("boom")
	assert.Same(t, err, recoveredPanicError(err))
}

func recoverPanic(fn func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()

	fn()
	return nil
}

func decodeRecoverLog(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	line := strings.TrimSpace(buffer.String())
	require.NotEmpty(t, line)

	entry := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(line), &entry))
	return entry
}

func TestRecoverMiddlewareUsesLoggerFromContext(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil)).With(slog.String("request_id", "req_123"))

	handler := RecoverMiddleware()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	}))

	request := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	request = request.WithContext(context.WithValue(request.Context(), struct{}{}, "unrelated")) // nolint
	request = request.WithContext(goforge.WithLogger(request.Context(), logger))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	entry := decodeRecoverLog(t, &logBuffer)
	assert.Equal(t, "req_123", entry["request_id"])
}
