package eventstream

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const eventstreamScope = "github.com/babelsuite/babelsuite/internal/eventstream"

var (
	esAppends     metric.Int64Counter
	esSubscribers metric.Int64UpDownCounter
)

func init() {
	meter := otel.Meter(eventstreamScope)
	esAppends, _ = meter.Int64Counter("babelsuite.eventstream.appends",
		metric.WithDescription("Events appended to streams"))
	esSubscribers, _ = meter.Int64UpDownCounter("babelsuite.eventstream.subscribers",
		metric.WithDescription("Active event stream subscribers"))
}

func recordESAppend(key string) {
	esAppends.Add(context.Background(), 1, metric.WithAttributes(attribute.String("stream", key)))
}

func recordESSubscribe(key string, delta int64) {
	esSubscribers.Add(context.Background(), delta, metric.WithAttributes(attribute.String("stream", key)))
}
