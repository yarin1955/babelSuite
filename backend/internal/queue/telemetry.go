package queue

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const queueScope = "github.com/babelsuite/babelsuite/internal/queue"

type queueSignals struct {
	tracer        trace.Tracer
	taskRuns      metric.Int64Counter
	taskFailures  metric.Int64Counter
	leaseExpiries metric.Int64Counter
}

func newQueueSignals() *queueSignals {
	meter := otel.Meter(queueScope)
	runs, _ := meter.Int64Counter("babelsuite.queue.task.runs",
		metric.WithDescription("Tasks executed by the queue"))
	failures, _ := meter.Int64Counter("babelsuite.queue.task.failures",
		metric.WithDescription("Tasks that completed with an error"))
	expiries, _ := meter.Int64Counter("babelsuite.queue.lease.expiries",
		metric.WithDescription("Tasks whose lease expired and were canceled"))
	return &queueSignals{
		tracer:        otel.Tracer(queueScope),
		taskRuns:      runs,
		taskFailures:  failures,
		leaseExpiries: expiries,
	}
}

var queueMetrics = newQueueSignals()

func registerQueueGauge(m *Memory) {
	meter := otel.Meter(queueScope)
	gauge, err := meter.Int64ObservableGauge("babelsuite.queue.depth",
		metric.WithDescription("Number of queue entries grouped by state"))
	if err != nil {
		return
	}
	meter.RegisterCallback(func(_ context.Context, obs metric.Observer) error { //nolint:errcheck
		for state, count := range m.stateTallies() {
			obs.ObserveInt64(gauge, count, metric.WithAttributes(
				attribute.String("queue.state", string(state)),
			))
		}
		return nil
	}, gauge)
}

func startTaskSpan(ctx context.Context, task Task) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("queue.task_id", task.ID),
		attribute.String("queue.group", task.Group),
		attribute.String("queue.name", task.Name),
	}
	spanCtx, span := queueMetrics.tracer.Start(ctx, "queue.task",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	queueMetrics.taskRuns.Add(ctx, 1, metric.WithAttributes(attrs...))
	return spanCtx, span
}

func finishTaskSpan(ctx context.Context, span trace.Span, task Task, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		queueMetrics.taskFailures.Add(ctx, 1, metric.WithAttributes(
			attribute.String("queue.group", task.Group),
		))
	}
	span.End()
}
