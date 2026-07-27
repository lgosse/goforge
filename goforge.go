// Package goforge defines the shared contracts and primitives used throughout
// the GoForge modules.
//
// It keeps service-facing concerns such as structured errors, JSON responses,
// context-scoped logging, and endpoint registration consistent without
// imposing an application framework. Specialized integrations live in
// independently versioned modules.
//
// A typical service uses chassis as its inbound HTTP foundation.
// chassis.NewServeMux starts with the standard library's routing behavior and
// adds only the logging, recovery, tracing, authentication, CORS, caching, or
// application middleware the service selects. The httpmiddlewares module
// exposes those building blocks directly when a service does not need the
// chassis abstraction.
//
// For outbound HTTP, httpclient.NewClient builds a standard http.Client whose
// transport can be composed with authentication and OpenTelemetry
// instrumentation. httpclient.Call can then form the typed JSON execution
// layer beneath small service-specific SDKs.
//
// The storage, messaging, error-reporting, and tooling modules complement
// these inbound and outbound foundations. Each module is independently
// versioned, so applications can adopt only the integrations they need while
// sharing the contracts defined by this root package.
package goforge
