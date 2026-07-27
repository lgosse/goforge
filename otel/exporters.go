package otel

import (
	"context"
	"crypto/tls"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

func newTraceExporter(ctx context.Context, config OTLPConfig) (sdktrace.SpanExporter, error) {
	if config.Protocol == ProtocolHTTP {
		options := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(config.Endpoint),
			otlptracehttp.WithHeaders(config.Headers),
			otlptracehttp.WithTimeout(config.Timeout),
			otlptracehttp.WithRetry(traceHTTPRetry(config.Retry)),
		}
		if config.Insecure {
			options = append(options, otlptracehttp.WithInsecure())
		} else {
			options = append(options, otlptracehttp.WithTLSClientConfig(tlsConfig(config)))
		}
		if config.Gzip {
			options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}

		return otlptracehttp.New(ctx, options...)
	}

	options := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.Endpoint),
		otlptracegrpc.WithHeaders(config.Headers),
		otlptracegrpc.WithTimeout(config.Timeout),
		otlptracegrpc.WithRetry(traceGRPCRetry(config.Retry)),
	}
	if config.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	} else {
		options = append(options, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig(config))))
	}
	if config.Gzip {
		options = append(options, otlptracegrpc.WithCompressor("gzip"))
	}

	return otlptracegrpc.New(ctx, options...)
}

func newMetricExporter(ctx context.Context, config OTLPConfig) (sdkmetric.Exporter, error) {
	if config.Protocol == ProtocolHTTP {
		options := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(config.Endpoint),
			otlpmetrichttp.WithHeaders(config.Headers),
			otlpmetrichttp.WithTimeout(config.Timeout),
			otlpmetrichttp.WithRetry(metricHTTPRetry(config.Retry)),
			otlpmetrichttp.WithTemporalitySelector(sdkmetric.DefaultTemporalitySelector),
			otlpmetrichttp.WithAggregationSelector(sdkmetric.DefaultAggregationSelector),
		}
		if config.Insecure {
			options = append(options, otlpmetrichttp.WithInsecure())
		} else {
			options = append(options, otlpmetrichttp.WithTLSClientConfig(tlsConfig(config)))
		}
		if config.Gzip {
			options = append(options, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
		}

		return otlpmetrichttp.New(ctx, options...)
	}

	options := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(config.Endpoint),
		otlpmetricgrpc.WithHeaders(config.Headers),
		otlpmetricgrpc.WithTimeout(config.Timeout),
		otlpmetricgrpc.WithRetry(metricGRPCRetry(config.Retry)),
		otlpmetricgrpc.WithTemporalitySelector(sdkmetric.DefaultTemporalitySelector),
		otlpmetricgrpc.WithAggregationSelector(sdkmetric.DefaultAggregationSelector),
	}
	if config.Insecure {
		options = append(options, otlpmetricgrpc.WithInsecure())
	} else {
		options = append(options, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig(config))))
	}
	if config.Gzip {
		options = append(options, otlpmetricgrpc.WithCompressor("gzip"))
	}

	return otlpmetricgrpc.New(ctx, options...)
}

func newLogExporter(ctx context.Context, config OTLPConfig) (sdklog.Exporter, error) {
	if config.Protocol == ProtocolHTTP {
		options := []otlploghttp.Option{
			otlploghttp.WithEndpoint(config.Endpoint),
			otlploghttp.WithHeaders(config.Headers),
			otlploghttp.WithTimeout(config.Timeout),
			otlploghttp.WithRetry(logHTTPRetry(config.Retry)),
		}
		if config.Insecure {
			options = append(options, otlploghttp.WithInsecure())
		} else {
			options = append(options, otlploghttp.WithTLSClientConfig(tlsConfig(config)))
		}
		if config.Gzip {
			options = append(options, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		}

		return otlploghttp.New(ctx, options...)
	}

	options := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(config.Endpoint),
		otlploggrpc.WithHeaders(config.Headers),
		otlploggrpc.WithTimeout(config.Timeout),
		otlploggrpc.WithRetry(logGRPCRetry(config.Retry)),
	}
	if config.Insecure {
		options = append(options, otlploggrpc.WithInsecure())
	} else {
		options = append(options, otlploggrpc.WithTLSCredentials(credentials.NewTLS(tlsConfig(config))))
	}
	if config.Gzip {
		options = append(options, otlploggrpc.WithCompressor("gzip"))
	}

	return otlploggrpc.New(ctx, options...)
}

func tlsConfig(config OTLPConfig) *tls.Config {
	if config.TLSConfig != nil {
		return config.TLSConfig.Clone()
	}

	return &tls.Config{MinVersion: tls.VersionTLS12}
}

func traceHTTPRetry(config RetryConfig) otlptracehttp.RetryConfig {
	return otlptracehttp.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}

func traceGRPCRetry(config RetryConfig) otlptracegrpc.RetryConfig {
	return otlptracegrpc.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}

func metricHTTPRetry(config RetryConfig) otlpmetrichttp.RetryConfig {
	return otlpmetrichttp.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}

func metricGRPCRetry(config RetryConfig) otlpmetricgrpc.RetryConfig {
	return otlpmetricgrpc.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}

func logHTTPRetry(config RetryConfig) otlploghttp.RetryConfig {
	return otlploghttp.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}

func logGRPCRetry(config RetryConfig) otlploggrpc.RetryConfig {
	return otlploggrpc.RetryConfig{
		Enabled:         config.Enabled,
		InitialInterval: config.InitialInterval,
		MaxInterval:     config.MaxInterval,
		MaxElapsedTime:  config.MaxElapsedTime,
	}
}
