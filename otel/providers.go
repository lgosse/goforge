package otel

import (
	"context"
	"fmt"

	contribruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newTracerProvider(
	ctx context.Context,
	config Config,
	res *resource.Resource,
) (*sdktrace.TracerProvider, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithRawSpanLimits(config.Traces.SpanLimits),
	}
	sampler := config.Traces.Sampler
	if sampler == nil {
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.Traces.SampleRatio))
	}
	options = append(options, sdktrace.WithSampler(sampler))

	exporters := make([]sdktrace.SpanExporter, 0, 2)
	if config.OTLP.Enabled && config.Traces.ExportOTLP {
		exporter, err := newTraceExporter(ctx, config.OTLP)
		if err != nil {
			return nil, fmt.Errorf("otel: create trace exporter: %w", err)
		}
		exporters = append(exporters, exporter)
	}
	if config.Traces.Exporter != nil {
		exporters = append(exporters, config.Traces.Exporter)
	}
	for _, exporter := range exporters {
		if config.Traces.Batch {
			options = append(options, sdktrace.WithBatcher(
				exporter,
				sdktrace.WithBatchTimeout(config.Traces.BatchTimeout),
				sdktrace.WithExportTimeout(config.Traces.ExportTimeout),
				sdktrace.WithMaxQueueSize(config.Traces.MaxQueueSize),
				sdktrace.WithMaxExportBatchSize(config.Traces.MaxExportBatchSize),
			))
		} else {
			options = append(options, sdktrace.WithSyncer(exporter))
		}
	}

	return sdktrace.NewTracerProvider(options...), nil
}

func newMeterProvider(
	ctx context.Context,
	config Config,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, error) {
	options := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithCardinalityLimit(config.Metrics.CardinalityLimit),
	}
	if config.OTLP.Enabled && config.Metrics.ExportOTLP {
		exporter, err := newMetricExporter(ctx, config.OTLP)
		if err != nil {
			return nil, fmt.Errorf("otel: create metric exporter: %w", err)
		}
		options = append(options, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(config.Metrics.ExportInterval),
			sdkmetric.WithTimeout(config.Metrics.ExportTimeout),
		)))
	}
	if config.Metrics.Reader != nil {
		options = append(options, sdkmetric.WithReader(config.Metrics.Reader))
	}

	provider := sdkmetric.NewMeterProvider(options...)
	if config.RuntimeMetrics.Enabled {
		if err := contribruntime.Start(
			contribruntime.WithMeterProvider(provider),
			contribruntime.WithMinimumReadMemStatsInterval(
				config.RuntimeMetrics.MinimumReadMemStatsInterval,
			),
		); err != nil {
			_ = provider.Shutdown(context.Background())

			return nil, fmt.Errorf("otel: start runtime metrics: %w", err)
		}
	}

	return provider, nil
}

func newLoggerProvider(
	ctx context.Context,
	config Config,
	res *resource.Resource,
) (*sdklog.LoggerProvider, error) {
	options := []sdklog.LoggerProviderOption{
		sdklog.WithResource(res),
		sdklog.WithAttributeCountLimit(config.Logs.AttributeCountLimit),
		sdklog.WithAttributeValueLengthLimit(config.Logs.AttributeValueLengthLimit),
	}
	exporters := make([]sdklog.Exporter, 0, 2)
	if config.OTLP.Enabled && config.Logs.ExportOTLP {
		exporter, err := newLogExporter(ctx, config.OTLP)
		if err != nil {
			return nil, fmt.Errorf("otel: create log exporter: %w", err)
		}
		exporters = append(exporters, exporter)
	}
	if config.Logs.Exporter != nil {
		exporters = append(exporters, config.Logs.Exporter)
	}
	for _, exporter := range exporters {
		if config.Logs.Batch {
			options = append(options, sdklog.WithProcessor(sdklog.NewBatchProcessor(
				exporter,
				sdklog.WithExportInterval(config.Logs.BatchInterval),
				sdklog.WithExportTimeout(config.Logs.ExportTimeout),
				sdklog.WithMaxQueueSize(config.Logs.MaxQueueSize),
				sdklog.WithExportMaxBatchSize(config.Logs.MaxExportBatchSize),
			)))
		} else {
			options = append(options, sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
		}
	}

	return sdklog.NewLoggerProvider(options...), nil
}
