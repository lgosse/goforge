package forgesentry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestNewLoggerDelegatesToWrappedHandler(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewLogger(slog.NewJSONHandler(&buffer, nil))

	logger.Info("hello", slog.String("component", "worker"))

	if !strings.Contains(buffer.String(), `"msg":"hello"`) {
		t.Fatalf("expected wrapped handler to receive log record, got %q", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"component":"worker"`) {
		t.Fatalf("expected wrapped handler to receive attrs, got %q", buffer.String())
	}
}

func TestNewLoggerCapturesErrorRecordToSentry(t *testing.T) {
	transport := configureTestSentry(t)
	var buffer bytes.Buffer
	logger := NewLogger(slog.NewJSONHandler(&buffer, nil)).
		With(slog.String("service", "api")).
		WithGroup("http_request").
		With(slog.String("route", "GET /users/{id}"))

	err := errors.New("database unavailable")
	logger.Error(
		"request failed",
		slog.Any("error", err),
		slog.Int("status", 500),
		slog.Group("user", slog.String("id", "user_123")),
	)

	event := transport.lastEvent(t)
	if len(event.Exception) != 1 {
		t.Fatalf("expected one exception, got %d", len(event.Exception))
	}
	if event.Exception[0].Value != "database unavailable" {
		t.Fatalf("expected original error to be captured, got %q", event.Exception[0].Value)
	}

	if got := event.Extra["service"]; got != "api" {
		t.Fatalf("expected service attr in extras, got %#v", got)
	}
	if got := event.Extra["http_request.route"]; got != "GET /users/{id}" {
		t.Fatalf("expected grouped logger attr in extras, got %#v", got)
	}
	if got := event.Extra["http_request.status"]; got != int64(500) {
		t.Fatalf("expected grouped record attr in extras, got %#v", got)
	}
	if got := event.Extra["http_request.user.id"]; got != "user_123" {
		t.Fatalf("expected nested group attr in extras, got %#v", got)
	}
	if got := event.Extra["http_request.error"]; got != "database unavailable" {
		t.Fatalf("expected error attr string in extras, got %#v", got)
	}

	slogContext := event.Contexts[slogSentryContext]
	if slogContext["message"] != "request failed" {
		t.Fatalf("expected log message in context, got %#v", slogContext["message"])
	}
	if slogContext["level"] != "ERROR" {
		t.Fatalf("expected log level in context, got %#v", slogContext["level"])
	}
}

func TestNewLoggerDoesNotCaptureNonErrorRecord(t *testing.T) {
	transport := configureTestSentry(t)
	logger := NewLogger(slog.NewTextHandler(&bytes.Buffer{}, nil))

	logger.Warn("warning", slog.Any("error", errors.New("not captured")))

	if events := transport.events(); len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}

func TestNewLoggerCapturesMessageWhenErrorAttrIsMissing(t *testing.T) {
	transport := configureTestSentry(t)
	logger := NewLogger(slog.NewTextHandler(&bytes.Buffer{}, nil))

	logger.Error("failed without attr", slog.String("component", "worker"))

	event := transport.lastEvent(t)
	if len(event.Exception) != 1 {
		t.Fatalf("expected one exception, got %d", len(event.Exception))
	}
	if event.Exception[0].Value != "failed without attr" {
		t.Fatalf("expected log message exception, got %q", event.Exception[0].Value)
	}
	if got := event.Extra["component"]; got != "worker" {
		t.Fatalf("expected attr in extras, got %#v", got)
	}
}

func TestNewLoggerUsesHubFromContext(t *testing.T) {
	globalTransport := configureTestSentry(t)
	contextTransport := &testTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: contextTransport,
	})
	if err != nil {
		t.Fatalf("failed to create sentry client: %v", err)
	}

	ctx := sentry.SetHubOnContext(context.Background(), sentry.NewHub(client, sentry.NewScope()))
	logger := NewLogger(slog.NewTextHandler(&bytes.Buffer{}, nil))

	logger.LogAttrs(ctx, slog.LevelError, "failed", slog.Any("error", errors.New("context hub error")))

	if got := len(contextTransport.events()); got != 1 {
		t.Fatalf("expected context hub event, got %d", got)
	}
	if got := len(globalTransport.events()); got != 0 {
		t.Fatalf("expected global hub to remain untouched, got %d events", got)
	}
}

func TestNewLoggerCapturesOriginalErrorStackTrace(t *testing.T) {
	transport := configureTestSentry(t)
	logger := NewLogger(slog.NewTextHandler(&bytes.Buffer{}, nil))
	err := newStackedError("with stack")

	logger.Error("failed", slog.Any("error", err))

	event := transport.lastEvent(t)
	if len(event.Exception) != 1 {
		t.Fatalf("expected one exception, got %d", len(event.Exception))
	}
	if event.Exception[0].Stacktrace == nil || len(event.Exception[0].Stacktrace.Frames) == 0 {
		t.Fatal("expected sentry to extract stacktrace from original error")
	}
}

func TestNewLoggerHandlesNilHandler(t *testing.T) {
	transport := configureTestSentry(t)
	logger := NewLogger(nil)

	logger.Error("nil handler fallback")

	event := transport.lastEvent(t)
	if event.Exception[0].Value != "nil handler fallback" {
		t.Fatalf("expected sentry event from nil handler fallback, got %q", event.Exception[0].Value)
	}
}

type testTransport struct {
	mu       sync.Mutex
	captured []*sentry.Event
}

type stackedError struct {
	message string
	stack   []stackFrame
}

type stackFrame struct {
	ProgramCounter uintptr
}

func newStackedError(message string) stackedError {
	var pcs [8]uintptr
	n := runtime.Callers(1, pcs[:])
	stack := make([]stackFrame, 0, n)
	for _, pc := range pcs[:n] {
		stack = append(stack, stackFrame{ProgramCounter: pc})
	}

	return stackedError{message: message, stack: stack}
}

func (e stackedError) Error() string {
	return e.message
}

func (e stackedError) StackTrace() []stackFrame {
	return append([]stackFrame(nil), e.stack...)
}

func (t *testTransport) Configure(_ sentry.ClientOptions) {}

func (t *testTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.captured = append(t.captured, event)
}

func (t *testTransport) Flush(_ time.Duration) bool {
	return true
}

func (t *testTransport) events() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.captured...)
}

func (t *testTransport) lastEvent(tb testing.TB) *sentry.Event {
	tb.Helper()

	events := t.events()
	if len(events) == 0 {
		tb.Fatal("expected sentry event")
	}

	return events[len(events)-1]
}

func configureTestSentry(t *testing.T) *testTransport {
	t.Helper()

	originalClient := sentry.CurrentHub().Client()
	transport := &testTransport{}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: transport,
	}); err != nil {
		t.Fatalf("failed to initialize sentry: %v", err)
	}
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(originalClient)
	})

	return transport
}

func TestSentryAttrValueFormatsJSONSafeValues(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 30, 0, 123, time.UTC)
	values := map[string]any{
		"duration": sentryAttrValue(slog.DurationValue(2 * time.Second)),
		"time":     sentryAttrValue(slog.TimeValue(now)),
	}

	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("expected attrs to be JSON serializable: %v", err)
	}
	if !strings.Contains(string(raw), "2s") {
		t.Fatalf("expected duration to be formatted, got %s", raw)
	}
	if !strings.Contains(string(raw), now.Format(time.RFC3339Nano)) {
		t.Fatalf("expected time to be formatted, got %s", raw)
	}
}

func TestSentryHandlerEnabledDelegates(t *testing.T) {
	handler := NewSentryHandler(slog.NewTextHandler(ioDiscard{}, &slog.HandlerOptions{Level: slog.LevelWarn}))

	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("expected wrapped handler to disable info")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("expected wrapped handler to enable error")
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
