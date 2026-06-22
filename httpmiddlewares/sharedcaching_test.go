package httpmiddlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestSharedCaching(t *testing.T) {
	restoreSharedCachingLogLevel(t)

	for _, d := range []struct {
		name                  string
		excludes              map[string][]string
		firstTarget           string
		firstHeaders          http.Header
		secondTarget          string
		secondHeaders         http.Header
		thirdTarget           string
		thirdHeaders          http.Header
		ageCachedEntryBy      time.Duration
		waitForRefresh        bool
		responseHeaders       http.Header
		responseStatus        int
		expectedBodies        []string
		expectedFreshnesses   []string
		expectedRevalidations []bool
		expectedHandlerCalls  int
		expectedStatusCode    int
	}{
		{
			name:         "caches public response and serves fresh hit",
			firstTarget:  "/test",
			secondTarget: "/test",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "caches explicit public non-2xx response",
			firstTarget:  "/missing",
			secondTarget: "/missing",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusNotFound,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusNotFound,
		},
		{
			name:         "request no-cache bypasses cached response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"no-cache"},
			},
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "request public plus no-cache still bypasses cached response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"public, no-cache"},
			},
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "request private plus no-cache still bypasses cached response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"private, no-cache"},
			},
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "request no-store does not store a fresh origin response",
			firstTarget:  "/test",
			firstHeaders: http.Header{"Cache-Control": {"no-store"}},
			secondTarget: "/test",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "request no-store still allows reuse of an already stored response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"no-store"},
			},
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request max-age rejects an entry older than the requested age",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"max-age=10"},
			},
			ageCachedEntryBy: 30 * time.Second,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "request public plus max-age still serves a matching cached response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"public, max-age=10"},
			},
			ageCachedEntryBy: 5 * time.Second,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request private plus max-age still serves a matching cached response",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"private, max-age=10"},
			},
			ageCachedEntryBy: 5 * time.Second,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request max-age accepts an entry within the requested age",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"max-age=10"},
			},
			ageCachedEntryBy: 5 * time.Second,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request max-stale allows a stale response without origin swr",
			firstTarget:  "/test",
			secondTarget: "/test",
			thirdTarget:  "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"max-stale=5"},
			},
			ageCachedEntryBy: 2 * time.Second,
			waitForRefresh:   true,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=1"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1", "call-2"},
			expectedFreshnesses:   []string{"stale", "fresh"},
			expectedRevalidations: []bool{true, false},
			expectedHandlerCalls:  2,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request max-stale without a value allows any stale response",
			firstTarget:  "/test",
			secondTarget: "/test",
			thirdTarget:  "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"max-stale"},
			},
			ageCachedEntryBy: 10 * time.Second,
			waitForRefresh:   true,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=1"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1", "call-2"},
			expectedFreshnesses:   []string{"stale", "fresh"},
			expectedRevalidations: []bool{true, false},
			expectedHandlerCalls:  2,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name:         "request max-stale still rejects stale entries beyond the requested limit",
			firstTarget:  "/test",
			secondTarget: "/test",
			secondHeaders: http.Header{
				"Cache-Control": {"max-stale=1"},
			},
			ageCachedEntryBy: 5 * time.Second,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=1"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "normalizes query parameter order in cache key",
			firstTarget:  "/test?b=2&a=1",
			secondTarget: "/test?a=1&b=2",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1"},
			expectedFreshnesses:   []string{"fresh"},
			expectedRevalidations: []bool{false},
			expectedHandlerCalls:  1,
			expectedStatusCode:    http.StatusOK,
		},
		{
			name: "bypasses excluded routes",
			excludes: map[string][]string{
				"/test": {http.MethodGet},
			},
			firstTarget:  "/test",
			secondTarget: "/test",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "does not cache private responses",
			firstTarget:  "/test",
			secondTarget: "/test",
			responseHeaders: http.Header{
				"Cache-Control": {"private, max-age=60"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:         "does not cache set-cookie responses",
			firstTarget:  "/test",
			secondTarget: "/test",
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=60"},
				"Set-Cookie":    {"session=abc"},
			},
			responseStatus:       http.StatusOK,
			expectedBodies:       []string{"call-1", "call-2"},
			expectedHandlerCalls: 2,
			expectedStatusCode:   http.StatusOK,
		},
		{
			name:             "serves stale while revalidating when the response allows it",
			firstTarget:      "/test",
			secondTarget:     "/test",
			thirdTarget:      "/test",
			ageCachedEntryBy: 2 * time.Second,
			waitForRefresh:   true,
			responseHeaders: http.Header{
				"Cache-Control": {"public, max-age=1, stale-while-revalidate=60"},
			},
			responseStatus:        http.StatusOK,
			expectedBodies:        []string{"call-1", "call-1", "call-2"},
			expectedFreshnesses:   []string{"stale", "fresh"},
			expectedRevalidations: []bool{true, false},
			expectedHandlerCalls:  2,
			expectedStatusCode:    http.StatusOK,
		},
	} {
		t.Run(d.name, func(t *testing.T) {
			store := newFakeSharedCacheStore()
			logger, logBuffer := newTestSharedCacheLogger()
			var handlerCalls atomic.Int32
			refreshDone := make(chan struct{}, 1)
			cacheKey := ""

			var opts middlewareOptions
			for pattern, methods := range d.excludes {
				for _, method := range methods {
					opts.excludes[method+" "+pattern] = struct{}{}
				}
			}
			middleware := sharedCaching(logger, "api.side.co", store, opts)

			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callNumber := handlerCalls.Add(1)
				for key, values := range d.responseHeaders {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(d.responseStatus)
				_, _ = fmt.Fprintf(w, "call-%d", callNumber)
			}))

			firstRequest := httptest.NewRequest(http.MethodGet, "http://example.com"+d.firstTarget, nil)
			for key, values := range d.firstHeaders {
				for _, value := range values {
					firstRequest.Header.Add(key, value)
				}
			}
			firstRecorder := httptest.NewRecorder()
			handler.ServeHTTP(firstRecorder, firstRequest)

			cacheKey = buildSharedCacheKey("api.side.co", firstRequest)
			if d.ageCachedEntryBy > 0 {
				entry := sharedCacheEntry{}
				store.mu.RLock()
				rawEntry := store.values[cacheKey]
				store.mu.RUnlock()
				if !assert.NoError(t, json.Unmarshal([]byte(rawEntry), &entry)) {
					return
				}
				entry.StoredAt = time.Now().Add(-d.ageCachedEntryBy)
				payload, err := json.Marshal(entry)
				if !assert.NoError(t, err) {
					return
				}
				store.mu.Lock()
				store.values[cacheKey] = string(payload)
				store.mu.Unlock()
			}

			if d.waitForRefresh {
				store.setHook = func(setKey string) {
					if setKey != cacheKey {
						return
					}

					select {
					case refreshDone <- struct{}{}:
					default:
					}
				}
			}

			secondRequest := httptest.NewRequest(http.MethodGet, "http://example.com"+d.secondTarget, nil)
			for key, values := range d.secondHeaders {
				for _, value := range values {
					secondRequest.Header.Add(key, value)
				}
			}
			secondRecorder := httptest.NewRecorder()
			handler.ServeHTTP(secondRecorder, secondRequest)

			assert.Equal(t, d.responseStatus, firstRecorder.Code)
			assert.Equal(t, d.expectedBodies[0], firstRecorder.Body.String())
			assert.Equal(t, d.expectedStatusCode, secondRecorder.Code)
			assert.Equal(t, d.expectedBodies[1], secondRecorder.Body.String())

			if d.waitForRefresh {
				select {
				case <-refreshDone:
				case <-time.After(2 * time.Second):
					t.Fatal("timed out waiting for cache refresh")
				}
			}

			var thirdRequest *http.Request
			if d.thirdTarget != "" {
				thirdRequest = httptest.NewRequest(http.MethodGet, "http://example.com"+d.thirdTarget, nil)
				for key, values := range d.thirdHeaders {
					for _, value := range values {
						thirdRequest.Header.Add(key, value)
					}
				}
				thirdRecorder := httptest.NewRecorder()
				handler.ServeHTTP(thirdRecorder, thirdRequest)
				assert.Equal(t, d.expectedStatusCode, thirdRecorder.Code)
				assert.Equal(t, d.expectedBodies[2], thirdRecorder.Body.String())
			}

			assert.Equal(t, d.expectedHandlerCalls, int(handlerCalls.Load()))

			logs := decodeSharedCacheLogs(t, logBuffer)
			if !assert.Len(t, logs, len(d.expectedFreshnesses)) {
				return
			}

			for idx, freshness := range d.expectedFreshnesses {
				request := secondRequest
				if idx > 0 && thirdRequest != nil {
					request = thirdRequest
				}
				assertSharedCacheHitLog(t, logs[idx], buildSharedCacheKey("api.side.co", request), freshness, d.expectedRevalidations[idx])
			}
		})
	}

	for _, d := range []struct {
		name              string
		prepareStore      func(store *fakeSharedCacheStore, request *http.Request)
		expectedOperation string
	}{
		{
			name: "lookup failure",
			prepareStore: func(store *fakeSharedCacheStore, _ *http.Request) {
				store.getErr = errors.New("redis unavailable")
			},
			expectedOperation: "lookup",
		},
		{
			name: "invalid cached payload",
			prepareStore: func(store *fakeSharedCacheStore, request *http.Request) {
				store.mu.Lock()
				store.values[buildSharedCacheKey("api.side.co", request)] = "not-json"
				store.mu.Unlock()
			},
			expectedOperation: "decode",
		},
		{
			name: "store failure",
			prepareStore: func(store *fakeSharedCacheStore, _ *http.Request) {
				store.setErr = errors.New("redis write failed")
			},
			expectedOperation: "store",
		},
	} {
		t.Run("logs errors/"+d.name, func(t *testing.T) {
			store := newFakeSharedCacheStore()
			logger, logBuffer := newTestSharedCacheLogger()

			middleware := sharedCaching(logger, "api.side.co", store, middlewareOptions{})

			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "public, max-age=60")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("call-1"))
			}))

			request := httptest.NewRequest(http.MethodGet, "http://example.com/test?a=1&b=2", nil)
			d.prepareStore(store, request)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "call-1", recorder.Body.String())

			logs := decodeSharedCacheLogs(t, logBuffer)
			if !assert.Len(t, logs, 1) {
				return
			}
			assertSharedCacheErrorLog(t, logs[0], buildSharedCacheKey("api.side.co", request), d.expectedOperation)
		})
	}
}

type fakeSharedCacheStore struct {
	mu      sync.RWMutex
	values  map[string]string
	ttls    map[string]time.Duration
	getErr  error
	setErr  error
	setHook func(key string)
}

func newFakeSharedCacheStore() *fakeSharedCacheStore {
	return &fakeSharedCacheStore{
		values: make(map[string]string),
		ttls:   make(map[string]time.Duration),
	}
}

func (s *fakeSharedCacheStore) Get(_ context.Context, key string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.values[key]
	if !ok {
		return "", redis.Nil
	}

	return value, nil
}

func (s *fakeSharedCacheStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	if s.setErr != nil {
		return s.setErr
	}

	s.mu.Lock()
	s.values[key] = value
	s.ttls[key] = ttl
	s.mu.Unlock()

	if s.setHook != nil {
		s.setHook(key)
	}

	return nil
}

func newTestSharedCacheLogger() (*slog.Logger, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buffer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	return logger, buffer
}

func decodeSharedCacheLogs(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()

	output := strings.TrimSpace(buffer.String())
	if output == "" {
		return nil
	}

	lines := strings.Split(output, "\n")
	logs := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		entry := map[string]any{}
		if !assert.NoError(t, json.Unmarshal([]byte(line), &entry)) {
			return nil
		}
		logs = append(logs, entry)
	}

	return logs
}

func assertSharedCacheHitLog(t *testing.T, entry map[string]any, cacheKey, freshness string, revalidationTriggered bool) {
	t.Helper()

	assert.Equal(t, "shared cache hit", entry["message"])
	assert.Equal(t, sharedCachingMiddlewareName, entry["middleware"])
	assert.Equal(t, cacheKey, entry["cache_key"])
	assert.Equal(t, "hit", entry["cache_state"])
	assert.Equal(t, freshness, entry["cache_freshness"])
	assert.Equal(t, revalidationTriggered, entry["cache_revalidation_triggered"])
	assert.Contains(t, entry, "cache_age_seconds")
	assert.Contains(t, entry, "cache_max_age_seconds")
	assert.Contains(t, entry, "cache_stale_while_revalidate_seconds")
}

func assertSharedCacheErrorLog(t *testing.T, entry map[string]any, cacheKey, operation string) {
	t.Helper()

	assert.Equal(t, "shared cache error", entry["message"])
	assert.Equal(t, sharedCachingMiddlewareName, entry["middleware"])
	assert.Equal(t, cacheKey, entry["cache_key"])
	assert.Equal(t, operation, entry["cache_operation"])
	assert.Contains(t, entry, "error")
}

func restoreSharedCachingLogLevel(t *testing.T) {
	t.Helper()

	previousLevel := zerolog.GlobalLevel()
	previousErrorMarshalFunc := zerolog.ErrorMarshalFunc
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	zerolog.ErrorMarshalFunc = func(err error) any {
		return err.Error()
	}
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(previousLevel)
		zerolog.ErrorMarshalFunc = previousErrorMarshalFunc
	})
}
