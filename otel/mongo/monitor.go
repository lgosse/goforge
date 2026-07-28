package otelmongo

import (
	"errors"

	forgeotel "github.com/lgosse/goforge/otel"
	"go.mongodb.org/mongo-driver/v2/event"
	upstreamotelmongo "go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
)

var errNilRuntime = errors.New("otelmongo: nil telemetry runtime")

// NewMonitor creates a MongoDB command monitor backed by runtime's explicit
// trace and metric providers.
//
// Options are forwarded to the upstream OpenTelemetry MongoDB
// instrumentation. Runtime providers are applied last and therefore cannot be
// replaced through opts. Full MongoDB command attributes remain disabled
// unless explicitly enabled through an upstream option.
func NewMonitor(
	runtime *forgeotel.Runtime,
	opts ...upstreamotelmongo.Option,
) (*event.CommandMonitor, error) {
	if runtime == nil {
		return nil, errNilRuntime
	}

	options := append([]upstreamotelmongo.Option(nil), opts...)
	options = append(
		options,
		upstreamotelmongo.WithTracerProvider(runtime.TracerProvider()),
		upstreamotelmongo.WithMeterProvider(runtime.MeterProvider()),
	)

	return upstreamotelmongo.NewMonitor(options...), nil
}
