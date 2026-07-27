// Package chassis assembles standard-library HTTP servers with GoForge
// middleware.
//
// A chassis starts as a lightweight http.ServeMux and can opt into logging,
// recovery, tracing, authentication, CORS, caching, and application-specific
// middleware through functional options.
package chassis
