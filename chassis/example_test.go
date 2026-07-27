package chassis_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/lgosse/goforge/httpmiddlewares"

	"github.com/lgosse/goforge/chassis"
)

func ExampleNewServeMux() {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	addHeader := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Served-By", "goforge")
			next.ServeHTTP(w, r)
		})
	}

	options := []chassis.Option{
		// Start with the recommended request logger and panic recovery.
		chassis.WithDefaultChassis(),

		// Enable server spans. A service using github.com/lgosse/goforge/otel
		// normally passes telemetry.HTTPServerOptions()... here.
		chassis.WithOpenTelemetry(),

		// Explicit options override their default-chassis counterparts.
		chassis.WithLogger(
			logger,
			httpmiddlewares.WithMuxPatternExclusion("GET /health"),
		),
		chassis.WithRecover(),

		chassis.WithCORS(httpmiddlewares.CORSConfig{
			AllowedOrigins: []string{"https://app.example.com"},
			AllowedMethods: []string{http.MethodGet, http.MethodPost},
		}),
		chassis.WithAPIKey("secret"),
		chassis.WithMiddleware(addHeader),
	}

	// Shared caching needs a Redis client and a reachable Redis server. With
	// github.com/redis/go-redis/v9 imported, its setup would look like:
	//
	// redisClient := redis.NewClient(&redis.Options{
	//     Addr: "localhost:6379",
	// })
	// defer redisClient.Close()
	// options = append(options, chassis.WithSharedCaching(
	//     logger,
	//     "users",
	//     redisClient,
	//     httpmiddlewares.WithMuxPatternInclusion("GET /users/{id}"),
	// ))

	mux := chassis.NewServeMux(options...)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "healthy")
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("X-Api-Key", "secret")
	mux.ServeHTTP(recorder, request)

	fmt.Println(recorder.Code)
	fmt.Println(recorder.Header().Get("X-Served-By"))
	fmt.Println(recorder.Body.String())
	// Output:
	// 200
	// goforge
	// healthy
}
