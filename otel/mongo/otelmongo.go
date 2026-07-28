// Package otelmongo connects MongoDB Go Driver v2 command monitoring to a
// GoForge OpenTelemetry runtime.
//
// NewMonitor uses the runtime's explicit trace and metric providers without
// installing global providers. Applications retain ownership of MongoDB client
// options, connection, health checks, and disconnection.
package otelmongo
