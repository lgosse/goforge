package otelmongo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	forgeotel "github.com/lgosse/goforge/otel"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	upstreamotelmongo "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestNewMonitorUsesRuntimeProviders(t *testing.T) {
	telemetry, traceExporter, metricReader := newTestRuntime(t)
	monitor, err := NewMonitor(
		telemetry,
		upstreamotelmongo.WithTracerProvider(tracenoop.NewTracerProvider()),
		upstreamotelmongo.WithMeterProvider(metricnoop.NewMeterProvider()),
	)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	ctx, parent := telemetry.TracerProvider().Tracer("test").Start(
		context.Background(),
		"request",
	)
	command := mongoCommand(t, "find", "users", "email", "private@example.com")
	monitor.Started(ctx, startedEvent(1, "find", command))
	monitor.Succeeded(ctx, succeededEvent(1, "find"))
	parent.End()

	spans := traceExporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want 2", len(spans))
	}

	var mongoSpan *tracetest.SpanStub
	for idx := range spans {
		if spans[idx].Name == "users.find" {
			mongoSpan = &spans[idx]
			break
		}
	}
	if mongoSpan == nil {
		t.Fatal("MongoDB span not exported")
	}
	if mongoSpan.Parent.SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf(
			"MongoDB parent span ID = %s, want %s",
			mongoSpan.Parent.SpanID(),
			parent.SpanContext().SpanID(),
		)
	}
	if got := spanAttribute(mongoSpan.Attributes, "db.namespace"); got != "app" {
		t.Fatalf("db.namespace = %q, want app", got)
	}
	if got := spanAttribute(mongoSpan.Attributes, "db.collection.name"); got != "users" {
		t.Fatalf("db.collection.name = %q, want users", got)
	}
	if got := spanAttribute(mongoSpan.Attributes, "db.query.text"); got != "" {
		t.Fatalf("db.query.text = %q, want command privacy by default", got)
	}

	var metrics metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if count := operationDurationCount(metrics); count != 1 {
		t.Fatalf("db.client.operation.duration count = %d, want 1", count)
	}
}

func TestNewMonitorForwardsOptions(t *testing.T) {
	telemetry, traceExporter, _ := newTestRuntime(t)
	monitor, err := NewMonitor(
		telemetry,
		upstreamotelmongo.WithSpanNameFormatter(func(event *event.CommandStartedEvent) string {
			return "mongodb." + event.CommandName
		}),
		upstreamotelmongo.WithCommandAttributeDisabled(false),
	)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	command := mongoCommand(t, "find", "users", "email", "private@example.com")
	monitor.Started(context.Background(), startedEvent(2, "find", command))
	monitor.Succeeded(context.Background(), succeededEvent(2, "find"))

	spans := traceExporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "mongodb.find" {
		t.Fatalf("span name = %q, want mongodb.find", spans[0].Name)
	}
	query := spanAttribute(spans[0].Attributes, "db.query.text")
	if !strings.Contains(query, "private@example.com") {
		t.Fatalf("db.query.text = %q, want explicitly enabled command", query)
	}
}

func TestNewMonitorRecordsFailure(t *testing.T) {
	telemetry, traceExporter, metricReader := newTestRuntime(t)
	monitor, err := NewMonitor(telemetry)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	command := mongoCommand(t, "insert", "events", "name", "Lucas")
	monitor.Started(context.Background(), startedEvent(3, "insert", command))
	monitor.Failed(context.Background(), failedEvent(3, "insert", errors.New("write failed")))

	spans := traceExporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("span status = %v, want error", spans[0].Status.Code)
	}

	var metrics metricdata.ResourceMetrics
	if err := metricReader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if count := operationDurationCount(metrics); count != 1 {
		t.Fatalf("db.client.operation.duration count = %d, want 1", count)
	}
}

func TestNewMonitorConcurrentCommands(t *testing.T) {
	telemetry, traceExporter, _ := newTestRuntime(t)
	monitor, err := NewMonitor(telemetry)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}
	command := mongoCommand(t, "find", "users", "active", true)

	var waitGroup sync.WaitGroup
	for requestID := int64(1); requestID <= 100; requestID++ {
		// Each goroutine simulates one independently completed driver command.
		id := requestID
		waitGroup.Go(func() {
			monitor.Started(context.Background(), startedEvent(id, "find", command))
			monitor.Succeeded(context.Background(), succeededEvent(id, "find"))
		})
	}
	waitGroup.Wait()

	if got := len(traceExporter.GetSpans()); got != 100 {
		t.Fatalf("exported spans = %d, want 100", got)
	}
}

func TestNewMonitorRejectsNilRuntime(t *testing.T) {
	monitor, err := NewMonitor(nil)
	if monitor != nil {
		t.Fatal("NewMonitor() monitor is non-nil")
	}
	if !errors.Is(err, errNilRuntime) {
		t.Fatalf("NewMonitor() error = %v, want %v", err, errNilRuntime)
	}
}

func newTestRuntime(
	t *testing.T,
) (*forgeotel.Runtime, *tracetest.InMemoryExporter, *sdkmetric.ManualReader) {
	t.Helper()

	traceExporter := tracetest.NewInMemoryExporter()
	metricReader := sdkmetric.NewManualReader()
	config := forgeotel.DefaultConfig("test-service", true)
	config.Traces.Exporter = traceExporter
	config.Metrics.Reader = metricReader
	config.Logs.Enabled = false
	config.Logs.ConsoleEnabled = false
	config.RuntimeMetrics.Enabled = false

	telemetry, err := forgeotel.New(context.Background(), config)
	if err != nil {
		t.Fatalf("otel.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	return telemetry, traceExporter, metricReader
}

func mongoCommand(
	t *testing.T,
	commandName string,
	collection string,
	field string,
	value any,
) bson.Raw {
	t.Helper()

	command, err := bson.Marshal(bson.D{
		{Key: commandName, Value: collection},
		{Key: "filter", Value: bson.D{{Key: field, Value: value}}},
	})
	if err != nil {
		t.Fatalf("bson.Marshal() error = %v", err)
	}

	return command
}

func startedEvent(requestID int64, commandName string, command bson.Raw) *event.CommandStartedEvent {
	return &event.CommandStartedEvent{
		Command:      command,
		DatabaseName: "app",
		CommandName:  commandName,
		RequestID:    requestID,
		ConnectionID: "localhost:27017",
	}
}

func succeededEvent(requestID int64, commandName string) *event.CommandSucceededEvent {
	return &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			Duration:     10 * time.Millisecond,
			CommandName:  commandName,
			DatabaseName: "app",
			RequestID:    requestID,
			ConnectionID: "localhost:27017",
		},
	}
}

func failedEvent(requestID int64, commandName string, err error) *event.CommandFailedEvent {
	return &event.CommandFailedEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			Duration:     10 * time.Millisecond,
			CommandName:  commandName,
			DatabaseName: "app",
			RequestID:    requestID,
			ConnectionID: "localhost:27017",
		},
		Failure: err,
	}
}

func spanAttribute(attributes []attribute.KeyValue, key attribute.Key) string {
	for _, value := range attributes {
		if value.Key == key {
			return value.Value.AsString()
		}
	}

	return ""
}

func operationDurationCount(metrics metricdata.ResourceMetrics) uint64 {
	for _, scope := range metrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "db.client.operation.duration" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[float64])
			if !ok {
				return 0
			}

			var count uint64
			for _, point := range histogram.DataPoints {
				count += point.Count
			}

			return count
		}
	}

	return 0
}
