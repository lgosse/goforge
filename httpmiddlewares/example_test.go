package httpmiddlewares_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/lgosse/goforge/httpmiddlewares"
)

func ExampleAPIKeyMiddleware() {
	protected := httpmiddlewares.APIKeyMiddleware("secret")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "authorized")
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Api-Key", "secret")
	recorder := httptest.NewRecorder()

	protected.ServeHTTP(recorder, request)

	fmt.Println(recorder.Code)
	fmt.Println(recorder.Body.String())
	// Output:
	// 200
	// authorized
}

func ExampleCORSMiddleware() {
	handler := httpmiddlewares.CORSMiddleware(httpmiddlewares.CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{http.MethodGet},
	})(http.NotFoundHandler())

	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	fmt.Println(recorder.Code)
	fmt.Println(recorder.Header().Get("Access-Control-Allow-Origin"))
	fmt.Println(recorder.Header().Get("Access-Control-Allow-Methods"))
	// Output:
	// 204
	// https://app.example.com
	// GET
}
