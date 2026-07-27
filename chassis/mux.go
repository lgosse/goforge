package chassis

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/lgosse/goforge/httpmiddlewares"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type middleware func(http.Handler) http.Handler

// Option configures a ServeMux.
type Option func(*serveMuxConfig)

type serveMuxConfig struct {
	openTelemetry *openTelemetryConfig
	logger        *loggerConfig
	recover       *recoverConfig
	cors          *corsConfig
	apiKey        *apiKeyConfig
	sharedCaching *sharedCachingConfig
	middlewares   []middleware
}

type openTelemetryConfig struct {
	options []otelhttp.Option
}

type loggerConfig struct {
	logger  *slog.Logger
	options []httpmiddlewares.MiddlewareOption
}

type recoverConfig struct {
	options []httpmiddlewares.MiddlewareOption
}

type corsConfig struct {
	config  httpmiddlewares.CORSConfig
	options []httpmiddlewares.MiddlewareOption
}

type apiKeyConfig struct {
	apiKey  string
	options []httpmiddlewares.MiddlewareOption
}

type sharedCachingConfig struct {
	logger      *slog.Logger
	subdomain   string
	redisClient *redis.Client
	options     []httpmiddlewares.MiddlewareOption
}

// ServeMux dispatches HTTP requests through configured goforge middlewares.
//
// Handlers are wrapped when they are registered so the standard library mux
// resolves the request pattern and path values before any middleware runs.
type ServeMux struct {
	mux           *http.ServeMux
	middlewares   []middleware
	corsPreflight http.Handler
}

// NewServeMux returns a new middleware-aware HTTP request multiplexer.
//
// With no options, the returned mux behaves like a naked [http.ServeMux].
func NewServeMux(opts ...Option) *ServeMux {
	var config serveMuxConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}

	middlewares := make([]middleware, 0, 6+len(config.middlewares))
	corsMiddlewareCount := 0
	if config.openTelemetry != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.OpenTelemetryMiddleware(
				config.openTelemetry.options...,
			),
		)
	}
	if config.logger != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.LoggerMiddleware(
				config.logger.logger,
				config.logger.options...,
			),
		)
	}
	if config.recover != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.RecoverMiddleware(config.recover.options...),
		)
	}
	if config.cors != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.CORSMiddleware(
				config.cors.config,
				config.cors.options...,
			),
		)
		corsMiddlewareCount = len(middlewares)
	}
	if config.apiKey != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.APIKeyMiddleware(
				config.apiKey.apiKey,
				config.apiKey.options...,
			),
		)
	}
	if config.sharedCaching != nil {
		middlewares = append(
			middlewares,
			httpmiddlewares.SharedCachingMiddleware(
				config.sharedCaching.logger,
				config.sharedCaching.subdomain,
				config.sharedCaching.redisClient,
				config.sharedCaching.options...,
			),
		)
	}
	middlewares = append(middlewares, config.middlewares...)

	serveMux := &ServeMux{
		mux:         http.NewServeMux(),
		middlewares: middlewares,
	}
	if corsMiddlewareCount > 0 {
		serveMux.corsPreflight = serveMux.mux
		for idx := corsMiddlewareCount - 1; idx >= 0; idx-- {
			serveMux.corsPreflight = middlewares[idx](serveMux.corsPreflight)
		}
	}

	return serveMux
}

// Handle registers handler for pattern after applying the configured
// middlewares.
func (m *ServeMux) Handle(pattern string, handler http.Handler) {
	if handler == nil {
		panic("http: nil handler")
	}

	for idx := len(m.middlewares) - 1; idx >= 0; idx-- {
		handler = m.middlewares[idx](handler)
	}

	m.mux.Handle(pattern, handler)
}

// HandleFunc registers handler for pattern after applying the configured
// middlewares.
func (m *ServeMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	if handler == nil {
		panic("http: nil handler")
	}

	m.Handle(pattern, handler)
}

// Handler returns the handler and pattern that match r.
func (m *ServeMux) Handler(r *http.Request) (http.Handler, string) {
	return m.mux.Handler(r)
}

// ServeHTTP dispatches r to the handler whose pattern most closely matches its
// URL.
func (m *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.corsPreflight != nil && r.Method == http.MethodOptions {
		requestedMethod := strings.TrimSpace(r.Header.Get("Access-Control-Request-Method"))
		if requestedMethod != "" {
			targetRequest := r.Clone(r.Context())
			targetRequest.Method = strings.ToUpper(requestedMethod)
			_, r.Pattern = m.mux.Handler(targetRequest)

			m.corsPreflight.ServeHTTP(w, r)
			return
		}
	}

	m.mux.ServeHTTP(w, r)
}

// WithOpenTelemetry enables OpenTelemetry server tracing and metrics. Options
// are forwarded directly to otelhttp; the otel module supplies explicit
// production-ready provider options.
func WithOpenTelemetry(opts ...otelhttp.Option) Option {
	return func(config *serveMuxConfig) {
		config.openTelemetry = &openTelemetryConfig{
			options: append([]otelhttp.Option(nil), opts...),
		}
	}
}

// WithDefaultChassis enables the request logger and panic recovery
// middlewares. Explicit logger or recovery options take precedence regardless
// of option order.
func WithDefaultChassis() Option {
	return func(config *serveMuxConfig) {
		if config.logger == nil {
			config.logger = &loggerConfig{logger: slog.Default()}
		}
		if config.recover == nil {
			config.recover = &recoverConfig{}
		}
	}
}

// WithLogger enables request-scoped logging. A nil logger uses [slog.Default].
func WithLogger(logger *slog.Logger, opts ...httpmiddlewares.MiddlewareOption) Option {
	return func(config *serveMuxConfig) {
		if logger == nil {
			logger = slog.Default()
		}

		config.logger = &loggerConfig{
			logger:  logger,
			options: append([]httpmiddlewares.MiddlewareOption(nil), opts...),
		}
	}
}

// WithRecover enables panic recovery.
func WithRecover(opts ...httpmiddlewares.MiddlewareOption) Option {
	return func(config *serveMuxConfig) {
		config.recover = &recoverConfig{
			options: append([]httpmiddlewares.MiddlewareOption(nil), opts...),
		}
	}
}

// WithCORS enables cross-origin request handling using config.
func WithCORS(cors httpmiddlewares.CORSConfig, opts ...httpmiddlewares.MiddlewareOption) Option {
	return func(config *serveMuxConfig) {
		config.cors = &corsConfig{
			config:  cors,
			options: append([]httpmiddlewares.MiddlewareOption(nil), opts...),
		}
	}
}

// WithAPIKey enables X-Api-Key authentication.
func WithAPIKey(apiKey string, opts ...httpmiddlewares.MiddlewareOption) Option {
	return func(config *serveMuxConfig) {
		config.apiKey = &apiKeyConfig{
			apiKey:  apiKey,
			options: append([]httpmiddlewares.MiddlewareOption(nil), opts...),
		}
	}
}

// WithSharedCaching enables Redis-backed shared response caching.
func WithSharedCaching(
	logger *slog.Logger,
	subdomain string,
	redisClient *redis.Client,
	opts ...httpmiddlewares.MiddlewareOption,
) Option {
	return func(config *serveMuxConfig) {
		if logger == nil {
			logger = slog.Default()
		}

		config.sharedCaching = &sharedCachingConfig{
			logger:      logger,
			subdomain:   subdomain,
			redisClient: redisClient,
			options:     append([]httpmiddlewares.MiddlewareOption(nil), opts...),
		}
	}
}

// WithMiddleware adds arbitrary HTTP middlewares after the configured goforge
// middlewares. Middlewares run in the order provided whenever the preceding
// goforge middlewares delegate to the next handler.
func WithMiddleware(middlewares ...func(http.Handler) http.Handler) Option {
	return func(config *serveMuxConfig) {
		for _, nextMiddleware := range middlewares {
			if nextMiddleware == nil {
				panic("chassis: nil middleware")
			}

			config.middlewares = append(config.middlewares, nextMiddleware)
		}
	}
}
