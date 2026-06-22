package goforge

import "net/http"

// Endpoint is the standard contract for all goforge endpoints.
type Endpoint interface {
	Scheme() string
	Host() string
	Method() string
	Path() string
	Headers() http.Header
}

// EndpointRegistry is the standard contract for goforge endpoint registry.
type EndpointRegistry interface {
	Register(endpoint Endpoint) error
	Start() error
}
