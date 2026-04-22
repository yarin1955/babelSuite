package logstream

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const logstreamScope = "github.com/babelsuite/babelsuite/internal/logstream"

type hubMetrics struct {
	appends     metric.Int64Counter
	subscribers metric.Int64UpDownCounter
}

func newHubMetrics() *hubMetrics {
	meter := otel.Meter(logstreamScope)
	appends, _ := meter.Int64Counter("babelsuite.logstream.appends",
		metric.WithDescription("Log lines appended to streams"))
	subs, _ := meter.Int64UpDownCounter("babelsuite.logstream.subscribers",
		metric.WithDescription("Active log stream subscribers"))
	return &hubMetrics{appends: appends, subscribers: subs}
}

var hubSignals = newHubMetrics()

func recordAppend(executionID string) {
	hubSignals.appends.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("execution_id", executionID)))
}

func recordSubscribe(executionID string, delta int64) {
	hubSignals.subscribers.Add(context.Background(), delta,
		metric.WithAttributes(attribute.String("execution_id", executionID)))
}
