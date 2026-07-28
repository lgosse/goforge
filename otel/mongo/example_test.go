package otelmongo_test

import (
	"context"
	"fmt"

	forgeotel "github.com/lgosse/goforge/otel"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"

	forgeotelmongo "github.com/lgosse/goforge/otel/mongo"
)

func ExampleNewMonitor() {
	config := forgeotel.DefaultConfig("users-api", true)
	config.Logs.ConsoleEnabled = false
	config.RuntimeMetrics.Enabled = false

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

	monitor, err := forgeotelmongo.NewMonitor(telemetry)
	if err != nil {
		fmt.Println(err)
		return
	}

	// The application still owns all MongoDB options and client lifecycle.
	clientOptions := mongooptions.Client().
		ApplyURI("mongodb://localhost:27017").
		SetMonitor(monitor)

	fmt.Println(clientOptions.Monitor != nil)
	// Output:
	// true
}
