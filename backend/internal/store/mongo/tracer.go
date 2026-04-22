package mongo

import (
	"context"
	"sync"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const mongoScope = "github.com/babelsuite/babelsuite/internal/store/mongo"

type mongoTracer struct {
	tracer trace.Tracer
	spans  sync.Map // requestID int64 → trace.Span
}

func newMongoTracer() *mongoTracer {
	return &mongoTracer{tracer: otel.Tracer(mongoScope)}
}

func (m *mongoTracer) monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started:   m.started,
		Succeeded: m.succeeded,
		Failed:    m.failed,
	}
}

func (m *mongoTracer) started(ctx context.Context, evt *event.CommandStartedEvent) {
	_, span := m.tracer.Start(ctx, "mongodb."+evt.CommandName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemMongoDB,
			semconv.DBNamespaceKey.String(evt.DatabaseName),
			semconv.DBOperationNameKey.String(evt.CommandName),
		),
	)
	m.spans.Store(evt.RequestID, span)
}

func (m *mongoTracer) succeeded(_ context.Context, evt *event.CommandSucceededEvent) {
	v, ok := m.spans.LoadAndDelete(evt.RequestID)
	if !ok {
		return
	}
	v.(trace.Span).End()
}

func (m *mongoTracer) failed(_ context.Context, evt *event.CommandFailedEvent) {
	v, ok := m.spans.LoadAndDelete(evt.RequestID)
	if !ok {
		return
	}
	span := v.(trace.Span)
	if evt.Failure != nil {
		span.RecordError(evt.Failure)
		span.SetStatus(codes.Error, evt.Failure.Error())
	}
	span.End()
}
