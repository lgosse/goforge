// Package otel constructs complete OpenTelemetry runtimes for GoForge
// services.
//
// A Runtime owns explicitly configured trace, metric, and log providers,
// resource attributes, W3C propagation, Go runtime metrics, and slog handlers.
// It exposes options that plug directly into the chassis and httpclient
// OpenTelemetry middleware without relying on global providers or environment
// configuration.
// The independently versioned otel/mongo module connects these providers to
// application-owned MongoDB Go Driver v2 clients.
//
// Use [DefaultConfig] with localDevelopment set to true for readable local
// logs and providers that generate valid trace identifiers without requiring
// an OTLP collector. Production defaults enable secure OTLP export and require
// an explicit collector endpoint.
package otel
