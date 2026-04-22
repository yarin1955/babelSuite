package runner

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const runnerScope = "github.com/babelsuite/babelsuite/internal/runner"

type runnerSignals struct {
	tracer   trace.Tracer
	runs     metric.Int64Counter
	failures metric.Int64Counter
}

func newRunnerSignals() *runnerSignals {
	meter := otel.Meter(runnerScope)
	runs, _ := meter.Int64Counter("babelsuite.runner.runs",
		metric.WithDescription("Step executions dispatched by the local runner"))
	failures, _ := meter.Int64Counter("babelsuite.runner.failures",
		metric.WithDescription("Step executions that completed with an error"))
	return &runnerSignals{
		tracer:   otel.Tracer(runnerScope),
		runs:     runs,
		failures: failures,
	}
}

var runnerMetrics = newRunnerSignals()

func startStepSpan(ctx context.Context, step StepSpec) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("runner.execution_id", step.ExecutionID),
		attribute.String("runner.suite_id", step.SuiteID),
		attribute.String("runner.node_id", step.Node.ID),
		attribute.String("runner.node_kind", step.Node.Kind),
		attribute.String("runner.backend", "local"),
	}
	spanCtx, span := runnerMetrics.tracer.Start(ctx, "runner.run",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	runnerMetrics.runs.Add(ctx, 1, metric.WithAttributes(attrs...))
	return spanCtx, span
}

func finishStepSpan(ctx context.Context, span trace.Span, step StepSpec, err error) {
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		attrs := []attribute.KeyValue{
			attribute.String("runner.node_kind", step.Node.Kind),
		}
		runnerMetrics.failures.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	span.End()
}
