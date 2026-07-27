package otel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	upstreamotel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Runtime owns a service's OpenTelemetry providers, propagation, and logging.
// It is safe for concurrent use.
type Runtime struct {
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
	loggerProvider log.LoggerProvider
	propagator     propagation.TextMapPropagator
	handler        slog.Handler
	logger         *slog.Logger
	errorHandler   func(error)

	traceSDK  *sdktrace.TracerProvider
	metricSDK *sdkmetric.MeterProvider
	logSDK    *sdklog.LoggerProvider

	shutdownOnce sync.Once
	shutdownErr  error
}

// New constructs an explicitly configured OpenTelemetry runtime.
func New(ctx context.Context, config Config) (*Runtime, error) {
	config = config.clone()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	res, err := newResource(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}

	runtime := &Runtime{
		tracerProvider: tracenoop.NewTracerProvider(),
		meterProvider:  metricnoop.NewMeterProvider(),
		loggerProvider: noop.NewLoggerProvider(),
		propagator:     newPropagator(config.Propagation),
		errorHandler:   config.ErrorHandler,
	}
	if config.Traces.Enabled {
		runtime.traceSDK, err = newTracerProvider(ctx, config, res)
		if err != nil {
			return nil, err
		}
		runtime.tracerProvider = runtime.traceSDK
	}
	if config.Metrics.Enabled {
		runtime.metricSDK, err = newMeterProvider(ctx, config, res)
		if err != nil {
			_ = runtime.Shutdown(context.Background())

			return nil, err
		}
		runtime.meterProvider = runtime.metricSDK
	}
	if config.Logs.Enabled {
		runtime.logSDK, err = newLoggerProvider(ctx, config, res)
		if err != nil {
			_ = runtime.Shutdown(context.Background())

			return nil, err
		}
		runtime.loggerProvider = runtime.logSDK
	}
	runtime.handler = newLogHandler(config, runtime)
	runtime.logger = slog.New(runtime.handler)

	return runtime, nil
}

// TracerProvider returns the runtime's explicit tracer provider.
func (r *Runtime) TracerProvider() trace.TracerProvider {
	return r.tracerProvider
}

// MeterProvider returns the runtime's explicit meter provider.
func (r *Runtime) MeterProvider() metric.MeterProvider {
	return r.meterProvider
}

// LoggerProvider returns the runtime's explicit OpenTelemetry logger provider.
func (r *Runtime) LoggerProvider() log.LoggerProvider {
	return r.loggerProvider
}

// Propagator returns the runtime's configured text-map propagator.
func (r *Runtime) Propagator() propagation.TextMapPropagator {
	return r.propagator
}

// Handler returns a slog handler that writes configured console and OTLP logs.
func (r *Runtime) Handler() slog.Handler {
	return r.handler
}

// Logger returns the runtime's preconfigured slog logger.
func (r *Runtime) Logger() *slog.Logger {
	return r.logger
}

// HTTPServerOptions returns options for chassis or otelhttp server middleware.
func (r *Runtime) HTTPServerOptions() []otelhttp.Option {
	return r.httpOptions()
}

// HTTPClientOptions returns options for httpclient or otelhttp transports.
func (r *Runtime) HTTPClientOptions() []otelhttp.Option {
	return r.httpOptions()
}

func (r *Runtime) httpOptions() []otelhttp.Option {
	return []otelhttp.Option{
		otelhttp.WithTracerProvider(r.tracerProvider),
		otelhttp.WithMeterProvider(r.meterProvider),
		otelhttp.WithPropagators(r.propagator),
	}
}

// ForceFlush immediately exports all pending telemetry.
func (r *Runtime) ForceFlush(ctx context.Context) error {
	var result error
	if r.logSDK != nil {
		result = errors.Join(result, r.logSDK.ForceFlush(ctx))
	}
	if r.metricSDK != nil {
		result = errors.Join(result, r.metricSDK.ForceFlush(ctx))
	}
	if r.traceSDK != nil {
		result = errors.Join(result, r.traceSDK.ForceFlush(ctx))
	}

	return result
}

// Shutdown flushes and releases every provider. Repeated calls return the first
// shutdown result.
func (r *Runtime) Shutdown(ctx context.Context) error {
	r.shutdownOnce.Do(func() {
		if r.logSDK != nil {
			r.shutdownErr = errors.Join(r.shutdownErr, r.logSDK.Shutdown(ctx))
		}
		if r.metricSDK != nil {
			r.shutdownErr = errors.Join(r.shutdownErr, r.metricSDK.Shutdown(ctx))
		}
		if r.traceSDK != nil {
			r.shutdownErr = errors.Join(r.shutdownErr, r.traceSDK.Shutdown(ctx))
		}
	})

	return r.shutdownErr
}

// InstallGlobals installs the runtime as the process-wide OpenTelemetry
// providers and returns an idempotent restore function.
func (r *Runtime) InstallGlobals() func() {
	previousTracer := upstreamotel.GetTracerProvider()
	previousMeter := upstreamotel.GetMeterProvider()
	previousPropagator := upstreamotel.GetTextMapPropagator()
	previousLogger := global.GetLoggerProvider()
	previousErrorHandler := upstreamotel.GetErrorHandler()

	upstreamotel.SetTracerProvider(r.tracerProvider)
	upstreamotel.SetMeterProvider(r.meterProvider)
	upstreamotel.SetTextMapPropagator(r.propagator)
	global.SetLoggerProvider(r.loggerProvider)
	if r.errorHandler != nil {
		upstreamotel.SetErrorHandler(upstreamotel.ErrorHandlerFunc(r.errorHandler))
	}

	return sync.OnceFunc(func() {
		upstreamotel.SetTracerProvider(previousTracer)
		upstreamotel.SetMeterProvider(previousMeter)
		upstreamotel.SetTextMapPropagator(previousPropagator)
		global.SetLoggerProvider(previousLogger)
		upstreamotel.SetErrorHandler(previousErrorHandler)
	})
}
