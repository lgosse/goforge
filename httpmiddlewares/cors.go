package httpmiddlewares

import (
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	headerOrigin                             = "Origin"
	headerVary                               = "Vary"
	headerAccessControlAllowOrigin           = "Access-Control-Allow-Origin"
	headerAccessControlAllowCredentials      = "Access-Control-Allow-Credentials"
	headerAccessControlAllowMethods          = "Access-Control-Allow-Methods"
	headerAccessControlAllowHeaders          = "Access-Control-Allow-Headers"
	headerAccessControlExposeHeaders         = "Access-Control-Expose-Headers"
	headerAccessControlMaxAge                = "Access-Control-Max-Age"
	headerAccessControlRequestMethod         = "Access-Control-Request-Method"
	headerAccessControlRequestHeaders        = "Access-Control-Request-Headers"
	headerAccessControlRequestPrivateNetwork = "Access-Control-Request-Private-Network"
	headerAccessControlAllowPrivateNetwork   = "Access-Control-Allow-Private-Network"
)

var defaultCORSAllowedMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// CORSConfig controls how CORSMiddleware answers browser cross-origin requests.
type CORSConfig struct {
	// AllowedOrigins contains exact origins, wildcard origins such as "*", or
	// path.Match-compatible patterns such as "https://*.example.com".
	AllowedOrigins []string

	// AllowOriginFunc allows callers to implement dynamic origin checks.
	AllowOriginFunc func(origin string) bool

	// AllowedMethods controls Access-Control-Allow-Methods on preflight responses.
	// When empty, common HTTP API methods are allowed.
	AllowedMethods []string

	// AllowedHeaders controls Access-Control-Allow-Headers on preflight responses.
	// When empty or containing "*", the requested headers are reflected.
	AllowedHeaders []string

	// ExposedHeaders controls Access-Control-Expose-Headers on actual responses.
	ExposedHeaders []string

	// AllowCredentials adds Access-Control-Allow-Credentials: true.
	AllowCredentials bool

	// AllowPrivateNetwork answers Access-Control-Request-Private-Network preflights.
	AllowPrivateNetwork bool

	// MaxAge controls Access-Control-Max-Age on preflight responses.
	MaxAge time.Duration

	// PreflightStatusCode controls the status code used for handled preflight
	// requests. When zero, http.StatusNoContent is used.
	PreflightStatusCode int

	// PassthroughPreflight allows the next handler to process valid preflight
	// requests after CORS headers have been written.
	PassthroughPreflight bool
}

// CORSMiddleware returns middleware that applies CORS response headers and
// handles browser preflight requests.
func CORSMiddleware(config CORSConfig, opts ...middlewareOption) func(http.Handler) http.Handler {
	var options middlewareOptions
	for _, opt := range opts {
		opt(&options)
	}

	methods := normalizeTokens(config.AllowedMethods, false)
	if len(methods) == 0 {
		methods = append([]string(nil), defaultCORSAllowedMethods...)
	}

	allowedMethodSet := makeTokenSet(methods)
	allowedHeaders := normalizeTokens(config.AllowedHeaders, true)
	allowAnyHeader := len(allowedHeaders) == 0 || slices.Contains(allowedHeaders, "*")
	allowedHeaderSet := makeTokenSet(allowedHeaders)
	exposedHeaders := normalizeTokens(config.ExposedHeaders, true)

	preflightStatusCode := config.PreflightStatusCode
	if preflightStatusCode == 0 {
		preflightStatusCode = http.StatusNoContent
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if options.shouldExclude(r) {
				next.ServeHTTP(w, r)
				return
			}

			origin := strings.TrimSpace(r.Header.Get(headerOrigin))
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowedOrigin, originAllowed := corsAllowedOrigin(config, origin)
			if !originAllowed {
				if isCORSPreflight(r) {
					http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
					return
				}

				next.ServeHTTP(w, r)
				return
			}

			isWildcardOrigin := allowedOrigin == "*"
			if isWildcardOrigin && config.AllowCredentials {
				allowedOrigin = origin
				isWildcardOrigin = false
			}

			writeCORSOriginHeaders(w.Header(), allowedOrigin, isWildcardOrigin, config.AllowCredentials, exposedHeaders)

			if !isCORSPreflight(r) {
				next.ServeHTTP(w, r)
				return
			}

			writeCORSPreflightVary(w.Header())

			requestedMethod := strings.ToUpper(strings.TrimSpace(r.Header.Get(headerAccessControlRequestMethod)))
			if requestedMethod == "" || !allowedMethodSet[requestedMethod] {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			requestedHeaders := parseHeaderTokens(r.Header.Get(headerAccessControlRequestHeaders), true)
			if !allowAnyHeader {
				for _, requestedHeader := range requestedHeaders {
					if !allowedHeaderSet[requestedHeader] {
						http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
						return
					}
				}
			}

			w.Header().Set(headerAccessControlAllowMethods, strings.Join(methods, ", "))
			if allowAnyHeader {
				if len(requestedHeaders) > 0 {
					w.Header().Set(headerAccessControlAllowHeaders, strings.Join(requestedHeaders, ", "))
				}
			} else if len(allowedHeaders) > 0 {
				w.Header().Set(headerAccessControlAllowHeaders, strings.Join(allowedHeaders, ", "))
			}

			if config.AllowPrivateNetwork && strings.EqualFold(strings.TrimSpace(r.Header.Get(headerAccessControlRequestPrivateNetwork)), "true") {
				w.Header().Set(headerAccessControlAllowPrivateNetwork, "true")
			}

			if maxAgeSeconds := int64(config.MaxAge / time.Second); maxAgeSeconds > 0 {
				w.Header().Set(headerAccessControlMaxAge, strconv.FormatInt(maxAgeSeconds, 10))
			}

			if config.PassthroughPreflight {
				next.ServeHTTP(w, r)
				return
			}

			w.WriteHeader(preflightStatusCode)
		})
	}
}

func corsAllowedOrigin(config CORSConfig, origin string) (string, bool) {
	for _, allowedOrigin := range config.AllowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		if allowedOrigin == "" {
			continue
		}
		if allowedOrigin == "*" {
			return "*", true
		}
		if allowedOrigin == origin {
			return origin, true
		}
		if matched, err := path.Match(allowedOrigin, origin); err == nil && matched {
			return origin, true
		}
	}

	if config.AllowOriginFunc != nil && config.AllowOriginFunc(origin) {
		return origin, true
	}

	return "", false
}

func writeCORSOriginHeaders(header http.Header, allowedOrigin string, isWildcardOrigin, allowCredentials bool, exposedHeaders []string) {
	header.Set(headerAccessControlAllowOrigin, allowedOrigin)
	if !isWildcardOrigin {
		appendVary(header, headerOrigin)
	}

	if allowCredentials {
		header.Set(headerAccessControlAllowCredentials, "true")
	}

	if len(exposedHeaders) > 0 {
		header.Set(headerAccessControlExposeHeaders, strings.Join(exposedHeaders, ", "))
	}
}

func writeCORSPreflightVary(header http.Header) {
	appendVary(header, headerAccessControlRequestMethod)
	appendVary(header, headerAccessControlRequestHeaders)
}

func isCORSPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get(headerAccessControlRequestMethod) != ""
}

func normalizeTokens(tokens []string, canonicalHeader bool) []string {
	normalized := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		for _, part := range parseHeaderTokens(token, canonicalHeader) {
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			normalized = append(normalized, part)
		}
	}

	return normalized
}

func parseHeaderTokens(raw string, canonicalHeader bool) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		if token == "*" {
			tokens = append(tokens, token)
			continue
		}
		if canonicalHeader {
			token = http.CanonicalHeaderKey(token)
		} else {
			token = strings.ToUpper(token)
		}
		tokens = append(tokens, token)
	}

	return tokens
}

func makeTokenSet(tokens []string) map[string]bool {
	set := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		set[token] = true
	}
	return set
}

func appendVary(header http.Header, values ...string) {
	existing := header.Values(headerVary)
	seen := make(map[string]struct{})
	for _, value := range existing {
		for token := range strings.SplitSeq(value, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				seen[strings.ToLower(token)] = struct{}{}
			}
		}
	}

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		header.Add(headerVary, value)
		seen[key] = struct{}{}
	}
}
