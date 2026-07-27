package httpclient_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/lgosse/goforge/httpclient"
)

func ExampleNewClient() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyOK := r.Header.Get("X-API-Key") == "secret"
		bearerOK := r.Header.Get("Authorization") == "Bearer access-token"
		if !apiKeyOK || !bearerOK {
			http.Error(w, "missing credentials", http.StatusUnauthorized)
			return
		}

		_, _ = io.WriteString(w, "API key and OAuth credentials received")
	}))
	defer server.Close()

	tokenProvider := func(context.Context) (httpclient.OAuthToken, error) {
		// A production provider would fetch or refresh this token through the
		// service's identity provider.
		return httpclient.OAuthToken{AccessToken: "access-token"}, nil
	}

	client := httpclient.NewClient(
		// Transport options wrap the transport configured before them.
		httpclient.WithTransport(http.DefaultTransport),
		httpclient.WithTimeout(5*time.Second),
		httpclient.WithAPIKey("X-API-Key", "secret"),
		httpclient.WithOAuth(tokenProvider, 30*time.Second),

		// This uses the global OpenTelemetry provider. Applications normally
		// configure their exporter and provider during process startup.
		httpclient.WithTelemetry(),
	)

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		nil,
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	response, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(client.Timeout)
	fmt.Println(string(body))
	// Output:
	// 5s
	// API key and OAuth credentials received
}

func ExampleCall() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(struct {
			ID       string `json:"id"`
			Page     string `json:"page"`
			APIKeyOK bool   `json:"apiKeyOK"`
		}{
			ID:       "user-1",
			Page:     r.URL.Query().Get("page"),
			APIKeyOK: r.Header.Get("X-API-Key") == "secret",
		})
	}))
	defer server.Close()

	client := httpclient.NewClient(
		httpclient.WithAPIKey("X-API-Key", "secret"),
	)
	response, err := httpclient.Call[struct {
		ID       string `json:"id"`
		Page     string `json:"page"`
		APIKeyOK bool   `json:"apiKeyOK"`
	}](
		context.Background(),
		client,
		http.MethodGet,
		server.URL,
		"/users",
		nil,
		&httpclient.RequestOpts{
			Query: url.Values{"page": {"2"}},
		},
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(response.ID, response.Page, response.APIKeyOK)
	// Output:
	// user-1 2 true
}
