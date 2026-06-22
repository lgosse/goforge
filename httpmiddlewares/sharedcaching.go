package httpmiddlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lgosse/goforge"
	"github.com/redis/go-redis/v9"
)

const (
	sharedCachingMiddlewareName = "sharedcaching"
)

type sharedCacheStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

type redisSharedCacheStore struct {
	client *redis.Client
}

func (s redisSharedCacheStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s redisSharedCacheStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

type sharedCacheEntry struct {
	Status                      int         `json:"status"`
	Header                      http.Header `json:"header"`
	Body                        []byte      `json:"body"`
	StoredAt                    time.Time   `json:"storedAt"`
	MaxAgeSeconds               int         `json:"maxAgeSeconds"`
	StaleWhileRevalidateSeconds int         `json:"staleWhileRevalidateSeconds"`
}

type sharedCachingResponseRecorder struct {
	header      http.Header
	body        bytes.Buffer
	statusCode  int
	wroteHeader bool
	flushed     bool
}

// SharedCachingMiddleware caches explicit public GET responses in Redis and serves stale entries while it revalidates them in the background.
//
// Example:
//
//	router.HandleFunc("/public/tasks", func(w http.ResponseWriter, r *http.Request) {
//		w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=30")
//		sidehttp.Respond(w, myrawpayload, nil)
//	})
func SharedCachingMiddleware(logger *slog.Logger, subdomain string, redisClient *redis.Client, opts ...middlewareOption) func(http.Handler) http.Handler {
	var options middlewareOptions
	for _, opt := range opts {
		opt(&options)
	}

	// #Caching is an optimization, so a missing Redis client must not break the route that would otherwise work without it.
	if redisClient == nil {
		logger := logger.With(
			slog.String("middleware", sharedCachingMiddlewareName),
			slog.String("subdomain", subdomain),
		)

		logger.Error("missing redis client")
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}

	return sharedCaching(logger, subdomain, redisSharedCacheStore{client: redisClient}, options)
}

func sharedCaching(logger *slog.Logger, subdomain string, store sharedCacheStore, opts middlewareOptions) mux.MiddlewareFunc {

	// #The store abstraction exists so tests can exercise the middleware without needing a real Redis server.
	if store == nil {
		logger.Error(
			"missing shared cache store",
			slog.String("middleware", sharedCachingMiddlewareName),
			slog.String("subdomain", subdomain),
		)
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// #Shared caching is only for safe public reads, so every excluded route or non-GET request must bypass it completely.
			if opts.shouldExclude(r) || r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			cacheKey := buildSharedCacheKey(subdomain, r)
			logger := goforge.LoggerFromContext(ctx).With(
				slog.String("middleware", sharedCachingMiddlewareName),
				slog.Group("cache", slog.String("key", cacheKey)),
			)

			requestNoCache := false
			requestNoStore := false
			hasRequestMaxAge := false
			requestMaxAgeSeconds := 0
			hasRequestMaxStale := false
			requestMaxStaleSeconds := 0
			requestAcceptsAnyStale := false

			// #Request cache directives let callers trade freshness against latency, so the cache must honor them before it reuses shared data.
			for rawDirective := range strings.SplitSeq(strings.Join(r.Header.Values("Cache-Control"), ","), ",") {
				directive := strings.TrimSpace(rawDirective)
				if directive == "" {
					continue
				}

				key, value, hasValue := strings.Cut(strings.ToLower(directive), "=")
				key = strings.TrimSpace(key)

				if !hasValue {
					switch key {
					case "no-cache":
						requestNoCache = true
					case "no-store":
						requestNoStore = true
					case "max-stale":
						hasRequestMaxStale = true
						requestAcceptsAnyStale = true
					case "max-age":
						hasRequestMaxAge = true
						logger.Error("max-age requires a strictly positive integer value", slog.Group("cache", slog.String("cache_operation", "request_policy")),
							slog.String("cache_operation", "request_policy"),
						)
					}
					continue
				}

				value = strings.Trim(strings.TrimSpace(value), `"`)
				seconds, err := strconv.Atoi(value)
				if err != nil {
					logger.Error(
						"invalid cache directive value",
						slog.Group("cache", slog.String("operation", "request_policy")),
						slog.String("directive", key),
						slog.String("value", value),
						slog.Any("error", err),
					)

					if key == "max-age" {
						hasRequestMaxAge = true
					}
					if key == "max-stale" {
						hasRequestMaxStale = true
					}
					continue
				}
				if seconds < 0 {
					logger.Error(
						"cache directive value must be non-negative",
						slog.Group("cache", slog.String("operation", "request_policy")),
						slog.String("directive", key),
						slog.String("value", value),
					)

					if key == "max-age" {
						hasRequestMaxAge = true
					}
					if key == "max-stale" {
						hasRequestMaxStale = true
					}
					continue
				}

				switch key {
				case "max-age":
					hasRequestMaxAge = true
					requestMaxAgeSeconds = seconds
				case "max-stale":
					hasRequestMaxStale = true
					requestMaxStaleSeconds = seconds
				}
			}

			// #The cache lookup happens before the handler so repeated public reads can skip all downstream work when a shared response already exists.
			rawEntry, err := store.Get(ctx, cacheKey)
			if err != nil && !errors.Is(err, redis.Nil) {
				logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "lookup")),
					slog.Any("error", err),
				)
			}

			if err == nil {
				var cachedEntry sharedCacheEntry

				// #Corrupted or unsafe cache entries are ignored so the request falls back to the handler instead of serving something untrustworthy.
				if err := json.Unmarshal([]byte(rawEntry), &cachedEntry); err != nil {
					logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "decode")),
						slog.Any("error", err),
					)
				} else if cachedEntry.MaxAgeSeconds < 0 || cachedEntry.StaleWhileRevalidateSeconds < 0 {
					logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "decode")),
						slog.Any("error", fmt.Errorf("cached shared response contains invalid cache lifetime metadata")),
					)
				} else if len(cachedEntry.Header.Values("Set-Cookie")) > 0 {
					logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "decode")),
						slog.Any("error", fmt.Errorf("cached shared response contains Set-Cookie header")),
					)
				} else {
					age := max(time.Since(cachedEntry.StoredAt), 0)
					staleFor := max(age-time.Duration(cachedEntry.MaxAgeSeconds)*time.Second, 0)

					freshness := ""
					shouldRevalidate := false
					freshWindow := time.Duration(cachedEntry.MaxAgeSeconds) * time.Second
					staleWindow := freshWindow + time.Duration(cachedEntry.StaleWhileRevalidateSeconds)*time.Second
					requestAllowsStale := hasRequestMaxStale && (requestAcceptsAnyStale || staleFor <= time.Duration(requestMaxStaleSeconds)*time.Second)

					// #Client directives can demand a newer representation than the shared cache has, so they can force a pass-through even when an entry exists.
					if requestNoCache || (hasRequestMaxAge && age > time.Duration(requestMaxAgeSeconds)*time.Second) {
						freshness = ""
					} else {
						switch {
						case age <= freshWindow:
							freshness = "fresh"
						case requestAllowsStale || (cachedEntry.StaleWhileRevalidateSeconds > 0 && age <= staleWindow):
							freshness = "stale"
							shouldRevalidate = true
						}
					}

					if freshness != "" {
						copyHeaders(w.Header(), cachedEntry.Header)

						statusCode := cachedEntry.Status
						if statusCode == 0 {
							statusCode = http.StatusOK
						}

						w.WriteHeader(statusCode)
						if len(cachedEntry.Body) > 0 {
							if _, err := w.Write(cachedEntry.Body); err != nil {
								logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "write_response")),
									slog.Any("error", err),
								)
								return
							}
						}

						logger.Info("shared cache hit", slog.Group("cache",
							slog.String("operation", "serve"),
							slog.String("state", "hit"),
							slog.String("freshness", freshness),
							slog.Int("age_seconds", int(age/time.Second)),
							slog.Int("max_age_seconds", cachedEntry.MaxAgeSeconds),
							slog.Int("stale_while_revalidate_seconds", cachedEntry.StaleWhileRevalidateSeconds),
							slog.Bool("revalidation_triggered", shouldRevalidate),
						))

						if shouldRevalidate && !requestNoStore {
							// #Serving stale data keeps latency flat while the background refresh prevents many callers from stampeding the source at once.
							go func(originalRequest *http.Request) {
								detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(originalRequest.Context()), 120*time.Second)
								defer cancel()

								refreshRecorder := newSharedCachingResponseRecorder()
								next.ServeHTTP(refreshRecorder, originalRequest.Clone(detachedCtx))
								storeSharedCacheResponse(detachedCtx, logger, store, cacheKey, refreshRecorder, "refresh_store", !requestNoStore)
							}(r)
						}

						return
					}
				}
			}

			// #The response is buffered first so the middleware can inspect the final headers before deciding whether it is safe to share.
			recorder := newSharedCachingResponseRecorder()
			next.ServeHTTP(recorder, r)
			storeSharedCacheResponse(ctx, logger, store, cacheKey, recorder, "store", !requestNoStore)

			copyHeaders(w.Header(), recorder.Header())
			w.WriteHeader(recorder.StatusCode())
			if recorder.body.Len() > 0 {
				if _, err := w.Write(recorder.body.Bytes()); err != nil {
					logger.Error("shared cache error", slog.Group("cache", slog.String("operation", "write_response")),
						slog.Any("error", err),
					)
					return
				}
			}
			if recorder.flushed {
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		})
	}
}

func buildSharedCacheKey(subdomain string, r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		path = r.URL.Path
	}
	normalizedQuery := ""
	if values := r.URL.Query(); len(values) > 0 {
		normalized := make(url.Values, len(values))
		keys := make([]string, 0, len(values))

		// #Normalizing query parameter order keeps equivalent requests on the same cache key instead of fragmenting the cache.
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			vals := append([]string(nil), values[key]...)
			sort.Strings(vals)
			normalized[key] = vals
		}

		normalizedQuery = normalized.Encode()
	}

	return strings.Join([]string{
		"sharedcache",
		subdomain,
		r.Method,
		path,
		normalizedQuery,
	}, ":")
}

func storeSharedCacheResponse(ctx context.Context, logger *slog.Logger, store sharedCacheStore, cacheKey string, recorder *sharedCachingResponseRecorder, operation string, allowStore bool) {
	// #A client's no-store request forbids this middleware from persisting the origin response, even when the response itself is cacheable.
	if !allowStore {
		return
	}

	rawCacheControl := strings.Join(recorder.Header().Values("Cache-Control"), ",")
	isPublic := false
	isPrivate := false
	isNoCache := false
	isNoStore := false
	hasMaxAge := false
	maxAgeSeconds := 0
	staleWhileRevalidateSeconds := 0

	// #Shared caching is opt-in, so only handlers that explicitly publish a public lifetime are ever stored here.
	for rawDirective := range strings.SplitSeq(rawCacheControl, ",") {
		directive := strings.TrimSpace(rawDirective)
		if directive == "" {
			continue
		}

		key, value, hasValue := strings.Cut(strings.ToLower(directive), "=")
		key = strings.TrimSpace(key)
		if !hasValue {
			switch key {
			case "public":
				isPublic = true
			case "private":
				isPrivate = true
			case "no-cache":
				isNoCache = true
			case "no-store":
				isNoStore = true
			}
			continue
		}

		value = strings.Trim(strings.TrimSpace(value), `"`)
		seconds, err := strconv.Atoi(value)
		if err != nil {
			logger.Error("shared cache error", slog.Group("cache", slog.String("operation", operation+"_policy")),
				slog.Any("error", fmt.Errorf("invalid %s value %q: %w", key, value, err)),
			)
			return
		}
		if seconds < 0 {
			logger.Error("shared cache error", slog.Group("cache", slog.String("operation", operation+"_policy")),
				slog.Any("error", fmt.Errorf("%s must be non-negative", key)),
			)
			return
		}

		if key == "max-age" {
			hasMaxAge = true
			maxAgeSeconds = seconds
		}
		if key == "stale-while-revalidate" {
			staleWhileRevalidateSeconds = seconds
		}
	}

	// #Private responses, streamed responses, and cookie-bearing responses must never be shared across callers.
	if recorder.flushed || len(recorder.Header().Values("Set-Cookie")) > 0 || !isPublic || isPrivate || isNoCache || isNoStore || !hasMaxAge {
		return
	}

	ttl := time.Duration(maxAgeSeconds+staleWhileRevalidateSeconds) * time.Second
	if ttl <= 0 {
		return
	}

	// #The cache stores the original status, headers, and body together so a cache hit behaves like the source handler did.
	entry := sharedCacheEntry{
		Status:                      recorder.StatusCode(),
		Header:                      recorder.Header().Clone(),
		Body:                        append([]byte(nil), recorder.body.Bytes()...),
		StoredAt:                    time.Now(),
		MaxAgeSeconds:               maxAgeSeconds,
		StaleWhileRevalidateSeconds: staleWhileRevalidateSeconds,
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		logger.Error("shared cache error", slog.Group("cache", slog.String("operation", operation)),
			slog.Any("error", err),
		)
		return
	}

	if err := store.Set(ctx, cacheKey, string(payload), ttl); err != nil {
		logger.Error("shared cache error", slog.Group("cache", slog.String("operation", operation)),
			slog.Any("error", err),
		)
	}
}

func newSharedCachingResponseRecorder() *sharedCachingResponseRecorder {
	return &sharedCachingResponseRecorder{
		header: make(http.Header),
	}
}

func (r *sharedCachingResponseRecorder) Header() http.Header {
	return r.header
}

func (r *sharedCachingResponseRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	return r.body.Write(body)
}

func (r *sharedCachingResponseRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}

	r.statusCode = statusCode
	r.wroteHeader = true
}

func (r *sharedCachingResponseRecorder) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	r.flushed = true
}

func (r *sharedCachingResponseRecorder) StatusCode() int {
	if r.statusCode == 0 {
		return http.StatusOK
	}

	return r.statusCode
}

func copyHeaders(dst, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
