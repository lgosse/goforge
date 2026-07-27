package otel

import (
	"testing"
)

func TestDefaultConfigLocalDevelopment(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "environment-collector:4317")
	t.Setenv("ENVIRONMENT", "production")

	config := DefaultConfig("test-service", true)

	if config.OTLP.Enabled {
		t.Fatal("local development unexpectedly enables OTLP")
	}
	if config.Traces.SampleRatio != 1 {
		t.Fatalf("sample ratio = %v, want 1", config.Traces.SampleRatio)
	}
	if config.Traces.Batch || config.Logs.Batch {
		t.Fatal("local development unexpectedly enables batching")
	}
	if config.Logs.ConsoleFormat != ConsoleFormatText {
		t.Fatalf("console format = %v, want text", config.Logs.ConsoleFormat)
	}
	if config.DeploymentEnvironment != "" {
		t.Fatalf(
			"deployment environment = %q, want no implicit environment",
			config.DeploymentEnvironment,
		)
	}
}

func TestDefaultConfigProduction(t *testing.T) {
	config := DefaultConfig("test-service", false)
	config.OTLP.Endpoint = "collector:4317"

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !config.OTLP.Enabled || !config.Traces.ExportOTLP ||
		!config.Metrics.ExportOTLP || !config.Logs.ExportOTLP {
		t.Fatal("production does not enable all OTLP signals")
	}
	if config.OTLP.Insecure {
		t.Fatal("production unexpectedly disables transport security")
	}
	if config.Logs.ConsoleFormat != ConsoleFormatJSON {
		t.Fatalf("console format = %v, want JSON", config.Logs.ConsoleFormat)
	}
}

func TestConfigValidateRequiresExplicitEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "environment-collector:4317")

	config := DefaultConfig("test-service", false)
	err := config.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want missing explicit endpoint")
	}
}
