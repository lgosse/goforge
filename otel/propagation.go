package otel

import "go.opentelemetry.io/otel/propagation"

func newPropagator(config PropagationConfig) propagation.TextMapPropagator {
	propagators := make([]propagation.TextMapPropagator, 0, 2)
	if config.TraceContext {
		propagators = append(propagators, propagation.TraceContext{})
	}
	if config.Baggage {
		propagators = append(propagators, propagation.Baggage{})
	}

	return propagation.NewCompositeTextMapPropagator(propagators...)
}
