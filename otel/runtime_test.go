package otel

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRuntimeProvidesEverySignal(t *testing.T) {
	traceExporter := tracetest.NewInMemoryExporter()
	metricReader := sdkmetric.NewManualReader()
	logExporter := &recordingLogExporter{}
	var output bytes.Buffer

	config := DefaultConfig("test-service", true)
	config.Traces.Exporter = traceExporter
	config.Metrics.Reader = metricReader
	config.Logs.Exporter = logExporter
	config.Logs.ConsoleWriter = &output
	config.RuntimeMetrics.Enabled = false

	telemetry, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if shutdownErr := telemetry.Shutdown(context.Background()); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	ctx, span := telemetry.TracerProvider().Tracer("test").Start(
		context.Background(),
		"operation",
	)
	counter, err := telemetry.MeterProvider().Meter("test").Int64Counter("requests")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(ctx, 1)
	telemetry.Logger().InfoContext(ctx, "completed")
	spanContext := span.SpanContext()
	span.End()

	if !spanContext.IsValid() {
		t.Fatal("span context is invalid")
	}
	if got := len(traceExporter.GetSpans()); got != 1 {
		t.Fatalf("exported spans = %d, want 1", got)
	}

	var metrics metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if len(metrics.ScopeMetrics) == 0 {
		t.Fatal("no metrics collected")
	}

	records := logExporter.Records()
	if len(records) != 1 {
		t.Fatalf("exported logs = %d, want 1", len(records))
	}
	if records[0].TraceID() != spanContext.TraceID() {
		t.Fatalf("log trace ID = %s, want %s", records[0].TraceID(), spanContext.TraceID())
	}
	if records[0].SpanID() != spanContext.SpanID() {
		t.Fatalf("log span ID = %s, want %s", records[0].SpanID(), spanContext.SpanID())
	}
	if !bytes.Contains(output.Bytes(), []byte("trace_id="+spanContext.TraceID().String())) {
		t.Fatal("console log does not contain trace_id")
	}
	if !bytes.Contains(output.Bytes(), []byte("span_id="+spanContext.SpanID().String())) {
		t.Fatal("console log does not contain span_id")
	}
}

func TestRuntimeShutdownIsIdempotent(t *testing.T) {
	config := DefaultConfig("test-service", true)
	config.Logs.ConsoleEnabled = false
	config.RuntimeMetrics.Enabled = false

	telemetry, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := telemetry.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}
	if err := telemetry.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

type recordingLogExporter struct {
	mutex   sync.Mutex
	records []sdklog.Record
}

func (e *recordingLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}

	return nil
}

func (e *recordingLogExporter) Shutdown(context.Context) error {
	return nil
}

func (e *recordingLogExporter) ForceFlush(context.Context) error {
	return nil
}

func (e *recordingLogExporter) Records() []sdklog.Record {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	records := make([]sdklog.Record, len(e.records))
	for i := range e.records {
		records[i] = e.records[i].Clone()
	}

	return records
}

var _ sdklog.Exporter = (*recordingLogExporter)(nil)
var _ log.LoggerProvider = (*sdklog.LoggerProvider)(nil)
