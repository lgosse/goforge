package otel_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	forgeotel "github.com/lgosse/goforge/otel"
)

func ExampleNew() {
	config := forgeotel.DefaultConfig("orders-api", true)
	config.Logs.ConsoleWriter = io.Discard

	telemetry, err := forgeotel.New(context.Background(), config)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}()

	ctx, span := telemetry.TracerProvider().Tracer("orders").Start(
		context.Background(),
		"create-order",
	)
	defer span.End()

	telemetry.Logger().InfoContext(ctx, "order created", "order_id", "order-42")
	fmt.Println(span.SpanContext().IsValid())
	// Output:
	// true
}

func ExampleRuntime_Logger() {
	var output bytes.Buffer
	config := forgeotel.DefaultConfig("orders-api", true)
	config.Logs.ConsoleWriter = &output

	telemetry, err := forgeotel.New(context.Background(), config)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}()

	ctx, span := telemetry.TracerProvider().Tracer("orders").Start(
		context.Background(),
		"create-order",
	)
	telemetry.Logger().InfoContext(ctx, "order created")
	span.End()

	fmt.Println(strings.Contains(output.String(), "msg=\"order created\""))
	fmt.Println(strings.Contains(output.String(), "trace_id="))
	fmt.Println(strings.Contains(output.String(), "span_id="))
	// Output:
	// true
	// true
	// true
}

func ExampleRuntime_HTTPServerOptions() {
	config := forgeotel.DefaultConfig("orders-api", true)
	config.Logs.ConsoleWriter = io.Discard

	telemetry, err := forgeotel.New(context.Background(), config)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		if err := telemetry.Shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "healthy")
	})
	server := httptest.NewServer(otelhttp.NewHandler(
		handler,
		"GET /health",
		telemetry.HTTPServerOptions()...,
	))
	defer server.Close()

	client := &http.Client{
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			telemetry.HTTPClientOptions()...,
		),
		Timeout: time.Second,
	}
	response, err := client.Get(server.URL)
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
	fmt.Println(string(body))
	// Output:
	// healthy
}
