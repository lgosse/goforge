package otel

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	defaultExportTimeout       = 10 * time.Second
	defaultRetryInitial        = 5 * time.Second
	defaultRetryMaximum        = 30 * time.Second
	defaultRetryElapsed        = time.Minute
	defaultTraceBatchTimeout   = 5 * time.Second
	defaultLogBatchInterval    = time.Second
	defaultMetricInterval      = time.Minute
	defaultLocalMetricInterval = 10 * time.Second
	defaultMetricTimeout       = 30 * time.Second
	defaultRuntimeReadInterval = 15 * time.Second
	defaultTraceQueueSize      = 2048
	defaultTraceBatchSize      = 512
	defaultLogQueueSize        = 2048
	defaultLogBatchSize        = 512
	defaultAttributeCountLimit = 128
	defaultEventCountLimit     = 128
	defaultLinkCountLimit      = 128
	defaultMetricCardinality   = 2000
)

// Protocol selects the OTLP transport used by all enabled signal exporters.
type Protocol uint8

const (
	// ProtocolGRPC exports OTLP telemetry over gRPC.
	ProtocolGRPC Protocol = iota
	// ProtocolHTTP exports OTLP telemetry over HTTP with protobuf payloads.
	ProtocolHTTP
)

// ConsoleFormat selects the format of logs written to the console.
type ConsoleFormat uint8

const (
	// ConsoleFormatText writes human-readable key-value logs.
	ConsoleFormatText ConsoleFormat = iota
	// ConsoleFormatJSON writes structured JSON logs.
	ConsoleFormatJSON
)

// Config contains the complete explicit configuration used to construct a
// Runtime.
type Config struct {
	// ServiceName identifies the service producing telemetry and is required.
	ServiceName string
	// ServiceVersion identifies the deployed service version.
	ServiceVersion string
	// ServiceNamespace groups related services.
	ServiceNamespace string
	// DeploymentEnvironment identifies the deployment environment when set.
	DeploymentEnvironment string
	// LocalDevelopment enables local defaults without reading environment
	// variables.
	LocalDevelopment bool
	// Attributes contains additional resource attributes.
	Attributes []attribute.KeyValue
	// OTLP configures network export shared by all signals.
	OTLP OTLPConfig
	// Traces configures trace creation and export.
	Traces TraceConfig
	// Metrics configures metric collection and export.
	Metrics MetricConfig
	// Logs configures console and OpenTelemetry log delivery.
	Logs LogConfig
	// Propagation configures distributed context propagation.
	Propagation PropagationConfig
	// RuntimeMetrics configures Go runtime metric collection.
	RuntimeMetrics RuntimeMetricsConfig
	// ErrorHandler receives asynchronous OpenTelemetry SDK errors when globals
	// are installed.
	ErrorHandler func(error)
}

// OTLPConfig configures OTLP exporters without consulting OTEL environment
// variables.
type OTLPConfig struct {
	// Enabled permits OTLP exporters for signals that enable ExportOTLP.
	Enabled bool
	// Protocol selects OTLP over gRPC or HTTP.
	Protocol Protocol
	// Endpoint is the collector host and port without a URL scheme.
	Endpoint string
	// Insecure disables transport security.
	Insecure bool
	// TLSConfig customizes secure exporter connections.
	TLSConfig *tls.Config
	// Headers contains metadata sent with every export request.
	Headers map[string]string
	// Timeout bounds each exporter request.
	Timeout time.Duration
	// Gzip enables gzip compression.
	Gzip bool
	// Retry configures transient exporter retries.
	Retry RetryConfig
}

// RetryConfig configures bounded exponential retries for OTLP exporters.
type RetryConfig struct {
	// Enabled enables retrying transient export errors.
	Enabled bool
	// InitialInterval is the first retry delay.
	InitialInterval time.Duration
	// MaxInterval bounds the delay between retries.
	MaxInterval time.Duration
	// MaxElapsedTime bounds the complete retry period.
	MaxElapsedTime time.Duration
}

// TraceConfig configures tracing and optional additional exporters.
type TraceConfig struct {
	// Enabled creates a trace provider.
	Enabled bool
	// ExportOTLP exports spans through the shared OTLP configuration.
	ExportOTLP bool
	// SampleRatio is used by the default parent-based ratio sampler.
	SampleRatio float64
	// Sampler overrides the default sampler when non-nil.
	Sampler sdktrace.Sampler
	// Exporter adds a custom span exporter when non-nil.
	Exporter sdktrace.SpanExporter
	// Batch uses asynchronous batch processors instead of simple processors.
	Batch bool
	// BatchTimeout controls how frequently queued spans are exported.
	BatchTimeout time.Duration
	// ExportTimeout bounds a batch export.
	ExportTimeout time.Duration
	// MaxQueueSize bounds queued spans.
	MaxQueueSize int
	// MaxExportBatchSize bounds one exported span batch.
	MaxExportBatchSize int
	// SpanLimits explicitly bounds span attributes, events, and links.
	SpanLimits sdktrace.SpanLimits
}

// MetricConfig configures metrics and optional additional readers.
type MetricConfig struct {
	// Enabled creates a meter provider.
	Enabled bool
	// ExportOTLP exports metrics through the shared OTLP configuration.
	ExportOTLP bool
	// Reader adds a custom metric reader when non-nil.
	Reader sdkmetric.Reader
	// ExportInterval controls periodic OTLP collection.
	ExportInterval time.Duration
	// ExportTimeout bounds metric collection and export.
	ExportTimeout time.Duration
	// CardinalityLimit bounds datapoints per instrument and collection.
	CardinalityLimit int
}

// LogConfig configures console output, the OpenTelemetry log bridge, and an
// optional additional exporter.
type LogConfig struct {
	// Enabled creates an OpenTelemetry logger provider and bridge.
	Enabled bool
	// ExportOTLP exports logs through the shared OTLP configuration.
	ExportOTLP bool
	// Exporter adds a custom log exporter when non-nil.
	Exporter sdklog.Exporter
	// Batch uses an asynchronous batch processor instead of a simple processor.
	Batch bool
	// BatchInterval controls how frequently queued logs are exported.
	BatchInterval time.Duration
	// ExportTimeout bounds a batch export.
	ExportTimeout time.Duration
	// MaxQueueSize bounds queued log records.
	MaxQueueSize int
	// MaxExportBatchSize bounds one exported log batch.
	MaxExportBatchSize int
	// AttributeCountLimit bounds attributes on each log record.
	AttributeCountLimit int
	// AttributeValueLengthLimit bounds string and byte attribute lengths.
	AttributeValueLengthLimit int
	// ConsoleEnabled writes logs to ConsoleWriter.
	ConsoleEnabled bool
	// ConsoleFormat selects text or JSON console output.
	ConsoleFormat ConsoleFormat
	// ConsoleWriter receives console logs and defaults to os.Stdout.
	ConsoleWriter io.Writer
	// Level is the minimum enabled console and OpenTelemetry log level.
	Level slog.Level
	// AddSource includes source locations in console and OpenTelemetry logs.
	AddSource bool
}

// PropagationConfig configures standard distributed context propagation.
type PropagationConfig struct {
	// TraceContext enables W3C Trace Context propagation.
	TraceContext bool
	// Baggage enables W3C Baggage propagation.
	Baggage bool
}

// RuntimeMetricsConfig configures Go runtime metric instrumentation.
type RuntimeMetricsConfig struct {
	// Enabled records conventional Go runtime metrics.
	Enabled bool
	// MinimumReadMemStatsInterval limits expensive runtime.ReadMemStats calls.
	MinimumReadMemStatsInterval time.Duration
}

// DefaultConfig returns explicit production or local-development defaults for
// serviceName.
func DefaultConfig(serviceName string, localDevelopment bool) Config {
	traceRatio := 0.1
	metricInterval := defaultMetricInterval
	batch := true
	exportOTLP := true
	consoleFormat := ConsoleFormatJSON
	insecure := false
	if localDevelopment {
		traceRatio = 1
		metricInterval = defaultLocalMetricInterval
		batch = false
		exportOTLP = false
		consoleFormat = ConsoleFormatText
		insecure = true
	}

	return Config{
		ServiceName:      serviceName,
		LocalDevelopment: localDevelopment,
		OTLP: OTLPConfig{
			Enabled:  exportOTLP,
			Protocol: ProtocolGRPC,
			Insecure: insecure,
			Timeout:  defaultExportTimeout,
			Headers:  map[string]string{},
			Retry: RetryConfig{
				Enabled:         true,
				InitialInterval: defaultRetryInitial,
				MaxInterval:     defaultRetryMaximum,
				MaxElapsedTime:  defaultRetryElapsed,
			},
		},
		Traces: TraceConfig{
			Enabled:            true,
			ExportOTLP:         exportOTLP,
			SampleRatio:        traceRatio,
			Batch:              batch,
			BatchTimeout:       defaultTraceBatchTimeout,
			ExportTimeout:      defaultExportTimeout,
			MaxQueueSize:       defaultTraceQueueSize,
			MaxExportBatchSize: defaultTraceBatchSize,
			SpanLimits:         defaultSpanLimits(),
		},
		Metrics: MetricConfig{
			Enabled:          true,
			ExportOTLP:       exportOTLP,
			ExportInterval:   metricInterval,
			ExportTimeout:    defaultMetricTimeout,
			CardinalityLimit: defaultMetricCardinality,
		},
		Logs: LogConfig{
			Enabled:                   true,
			ExportOTLP:                exportOTLP,
			Batch:                     batch,
			BatchInterval:             defaultLogBatchInterval,
			ExportTimeout:             defaultExportTimeout,
			MaxQueueSize:              defaultLogQueueSize,
			MaxExportBatchSize:        defaultLogBatchSize,
			AttributeCountLimit:       defaultAttributeCountLimit,
			AttributeValueLengthLimit: -1,
			ConsoleEnabled:            true,
			ConsoleFormat:             consoleFormat,
			ConsoleWriter:             os.Stdout,
			Level:                     slog.LevelInfo,
		},
		Propagation: PropagationConfig{
			TraceContext: true,
			Baggage:      true,
		},
		RuntimeMetrics: RuntimeMetricsConfig{
			Enabled:                     true,
			MinimumReadMemStatsInterval: defaultRuntimeReadInterval,
		},
		ErrorHandler: func(err error) {
			_, _ = fmt.Fprintf(os.Stderr, "OpenTelemetry: %v\n", err)
		},
	}
}

// Validate verifies that the configuration can construct a Runtime.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ServiceName) == "" {
		return errors.New("otel: service name is required")
	}
	if c.OTLP.Protocol != ProtocolGRPC && c.OTLP.Protocol != ProtocolHTTP {
		return fmt.Errorf("otel: unsupported OTLP protocol %d", c.OTLP.Protocol)
	}
	if c.requiresOTLP() {
		if strings.TrimSpace(c.OTLP.Endpoint) == "" {
			return errors.New("otel: OTLP endpoint is required when export is enabled")
		}
		if strings.Contains(c.OTLP.Endpoint, "://") {
			return errors.New("otel: OTLP endpoint must not include a URL scheme")
		}
		if c.OTLP.Timeout <= 0 {
			return errors.New("otel: OTLP timeout must be positive")
		}
		if err := c.OTLP.Retry.validate(); err != nil {
			return err
		}
	}
	if c.Traces.Enabled {
		if c.Traces.SampleRatio < 0 || c.Traces.SampleRatio > 1 {
			return errors.New("otel: trace sample ratio must be between zero and one")
		}
		if c.Traces.Batch && (c.Traces.BatchTimeout <= 0 ||
			c.Traces.ExportTimeout <= 0 ||
			c.Traces.MaxQueueSize <= 0 ||
			c.Traces.MaxExportBatchSize <= 0) {
			return errors.New("otel: trace batch settings must be positive")
		}
	}
	if c.Metrics.Enabled {
		if c.Metrics.ExportInterval <= 0 || c.Metrics.ExportTimeout <= 0 {
			return errors.New("otel: metric export durations must be positive")
		}
		if c.Metrics.CardinalityLimit <= 0 {
			return errors.New("otel: metric cardinality limit must be positive")
		}
	}
	if c.Logs.Enabled {
		if c.Logs.ConsoleFormat != ConsoleFormatText &&
			c.Logs.ConsoleFormat != ConsoleFormatJSON {
			return fmt.Errorf("otel: unsupported console format %d", c.Logs.ConsoleFormat)
		}
		if c.Logs.ConsoleEnabled && c.Logs.ConsoleWriter == nil {
			return errors.New("otel: console writer is required when console logging is enabled")
		}
		if c.Logs.AttributeCountLimit <= 0 {
			return errors.New("otel: log attribute count limit must be positive")
		}
		if c.Logs.Batch && (c.Logs.BatchInterval <= 0 ||
			c.Logs.ExportTimeout <= 0 ||
			c.Logs.MaxQueueSize <= 0 ||
			c.Logs.MaxExportBatchSize <= 0) {
			return errors.New("otel: log batch settings must be positive")
		}
	}
	if c.RuntimeMetrics.Enabled && c.RuntimeMetrics.MinimumReadMemStatsInterval <= 0 {
		return errors.New("otel: runtime metric interval must be positive")
	}

	return nil
}

func (c Config) requiresOTLP() bool {
	if !c.OTLP.Enabled {
		return false
	}

	return c.Traces.Enabled && c.Traces.ExportOTLP ||
		c.Metrics.Enabled && c.Metrics.ExportOTLP ||
		c.Logs.Enabled && c.Logs.ExportOTLP
}

func (c Config) clone() Config {
	cloned := c
	cloned.Attributes = append([]attribute.KeyValue(nil), c.Attributes...)
	cloned.OTLP.Headers = cloneStringMap(c.OTLP.Headers)
	if c.OTLP.TLSConfig != nil {
		cloned.OTLP.TLSConfig = c.OTLP.TLSConfig.Clone()
	}

	return cloned
}

func (c RetryConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.InitialInterval <= 0 || c.MaxInterval <= 0 || c.MaxElapsedTime <= 0 {
		return errors.New("otel: retry durations must be positive")
	}
	if c.MaxInterval < c.InitialInterval {
		return errors.New("otel: retry max interval must not be shorter than initial interval")
	}

	return nil
}

func defaultSpanLimits() sdktrace.SpanLimits {
	return sdktrace.SpanLimits{
		AttributeValueLengthLimit:   -1,
		AttributeCountLimit:         defaultAttributeCountLimit,
		EventCountLimit:             defaultEventCountLimit,
		LinkCountLimit:              defaultLinkCountLimit,
		AttributePerEventCountLimit: defaultAttributeCountLimit,
		AttributePerLinkCountLimit:  defaultAttributeCountLimit,
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}

	return cloned
}
