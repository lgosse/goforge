package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func newResource(ctx context.Context, config Config) (*resource.Resource, error) {
	attributes := []attribute.KeyValue{semconv.ServiceName(config.ServiceName)}
	if config.ServiceVersion != "" {
		attributes = append(attributes, semconv.ServiceVersion(config.ServiceVersion))
	}
	if config.ServiceNamespace != "" {
		attributes = append(attributes, semconv.ServiceNamespace(config.ServiceNamespace))
	}
	if config.DeploymentEnvironment != "" {
		attributes = append(
			attributes,
			semconv.DeploymentEnvironmentNameKey.String(config.DeploymentEnvironment),
		)
	}
	attributes = append(attributes, config.Attributes...)

	return resource.New(
		ctx,
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(attributes...),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithOS(),
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
}
