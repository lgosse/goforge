package httpmiddlewares_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/lgosse/goforge/httpmiddlewares"
)

func TestOpenTelemetryMiddlewareInstrumentsRequest(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shut down tracer provider: %v", err)
		}
	})

	var spanContext trace.SpanContext
	handler := httpmiddlewares.OpenTelemetryMiddleware(
		otelhttp.WithTracerProvider(tracerProvider),
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spanContext = trace.SpanContextFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	request.Pattern = "GET /tasks/{id}"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
	if !spanContext.IsValid() {
		t.Fatal("expected a valid span context in the handler")
	}

	spans := spanRecorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected one ended span, got %d", len(spans))
	}
	if spans[0].Name() != "GET /tasks/{id}" {
		t.Fatalf("expected route-based span name %q, got %q", "GET /tasks/{id}", spans[0].Name())
	}
}

func TestOpenTelemetryMiddlewareForwardsOptions(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(ctx); err != nil {
			t.Fatalf("failed to shut down tracer provider: %v", err)
		}
	})

	handler := httpmiddlewares.OpenTelemetryMiddleware(
		otelhttp.WithTracerProvider(tracerProvider),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.Pattern != "GET /health"
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Pattern = "GET /health"
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if spans := spanRecorder.Ended(); len(spans) != 0 {
		t.Fatalf("expected filtered request not to create a span, got %d", len(spans))
	}
}
