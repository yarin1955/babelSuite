package mocking

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const mockingScope = "github.com/babelsuite/babelsuite/internal/mocking"

type mockingSignals struct {
	tracer      trace.Tracer
	invocations metric.Int64Counter
}

func newMockingSignals() *mockingSignals {
	meter := otel.Meter(mockingScope)
	fallback := metricnoop.NewMeterProvider().Meter(mockingScope)

	invocations, err := meter.Int64Counter(
		"babelsuite.mock.invocations",
		metric.WithDescription("Number of mock operation invocations resolved by the service."),
	)
	if err != nil {
		invocations, _ = fallback.Int64Counter("babelsuite.mock.invocations")
	}

	return &mockingSignals{
		tracer:      otel.Tracer(mockingScope),
		invocations: invocations,
	}
}

func (s *mockingSignals) start(ctx context.Context, suiteID, surfaceID, operationID, adapter string) (context.Context, trace.Span) {
	return s.tracer.Start(ctx, "mock.resolve",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("babelsuite.suite.id", normalizeMockValue(suiteID)),
			attribute.String("babelsuite.surface.id", normalizeMockValue(surfaceID)),
			attribute.String("babelsuite.operation.id", normalizeMockValue(operationID)),
			attribute.String("babelsuite.mock.adapter", normalizeMockValue(adapter)),
		),
	)
}

func (s *mockingSignals) finish(ctx context.Context, span trace.Span, result *Result, err error) {
	if span == nil {
		return
	}

	statusCode := 0
	matchedExample := ""
	if result != nil {
		statusCode = result.Status
		matchedExample = result.MatchedExample
	}

	spanAttrs := []attribute.KeyValue{
		attribute.Int("http.status_code", statusCode),
	}
	if matchedExample != "" {
		spanAttrs = append(spanAttrs, attribute.String("babelsuite.mock.matched_example", matchedExample))
	}
	span.SetAttributes(spanAttrs...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()

	s.invocations.Add(ctx, 1, metric.WithAttributes(
		attribute.Int("http.status_code", statusCode),
	))
}

func normalizeMockValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}
