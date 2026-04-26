package auth

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const authScope = "github.com/babelsuite/babelsuite/internal/auth"

type authSignals struct {
	tracer  trace.Tracer
	signIns metric.Int64Counter
	signUps metric.Int64Counter
}

func newAuthSignals() *authSignals {
	meter := otel.Meter(authScope)
	signIns, _ := meter.Int64Counter("babelsuite.auth.sign_ins",
		metric.WithDescription("Sign-in attempts by result"))
	signUps, _ := meter.Int64Counter("babelsuite.auth.sign_ups",
		metric.WithDescription("Sign-up attempts by result"))
	return &authSignals{
		tracer:  otel.Tracer(authScope),
		signIns: signIns,
		signUps: signUps,
	}
}

var authMetrics = newAuthSignals()

func resultAttr(success bool) attribute.KeyValue {
	if success {
		return attribute.String("result", "success")
	}
	return attribute.String("result", "failure")
}
