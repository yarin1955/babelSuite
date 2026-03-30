package execution

import (
	"context"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

const executionScope = "github.com/babelsuite/babelsuite/internal/execution"

type liveSpan struct {
	ctx  context.Context
	span trace.Span
}

type telemetrySet struct {
	tracer trace.Tracer

	launchCount  metric.Int64Counter
	rejectCount  metric.Int64Counter
	finishCount  metric.Int64Counter
	runDuration  metric.Float64Histogram
	stepDuration metric.Float64Histogram
	stateGauge   metric.Int64ObservableGauge
	registration metric.Registration
}

func newTelemetrySet(service *Service) *telemetrySet {
	meter := otel.Meter(executionScope)
	fallback := metricnoop.NewMeterProvider().Meter(executionScope)

	launchCount, err := meter.Int64Counter(
		"babelsuite.execution.created",
		metric.WithDescription("Number of suite execution requests accepted by the service."),
	)
	if err != nil {
		launchCount, _ = fallback.Int64Counter("babelsuite.execution.created")
	}

	rejectCount, err := meter.Int64Counter(
		"babelsuite.execution.rejected",
		metric.WithDescription("Number of suite execution requests rejected before completion."),
	)
	if err != nil {
		rejectCount, _ = fallback.Int64Counter("babelsuite.execution.rejected")
	}

	finishCount, err := meter.Int64Counter(
		"babelsuite.execution.finished",
		metric.WithDescription("Number of suite executions that reached a terminal status."),
	)
	if err != nil {
		finishCount, _ = fallback.Int64Counter("babelsuite.execution.finished")
	}

	runDuration, err := meter.Float64Histogram(
		"babelsuite.execution.duration.seconds",
		metric.WithDescription("Elapsed time for a suite execution."),
		metric.WithUnit("s"),
	)
	if err != nil {
		runDuration, _ = fallback.Float64Histogram("babelsuite.execution.duration.seconds")
	}

	stepDuration, err := meter.Float64Histogram(
		"babelsuite.execution.step.duration.seconds",
		metric.WithDescription("Elapsed time for an execution step."),
		metric.WithUnit("s"),
	)
	if err != nil {
		stepDuration, _ = fallback.Float64Histogram("babelsuite.execution.step.duration.seconds")
	}

	stateGauge, err := meter.Int64ObservableGauge(
		"babelsuite.execution.current",
		metric.WithDescription("Current in-memory execution records grouped by status."),
	)
	if err != nil {
		stateGauge, _ = fallback.Int64ObservableGauge("babelsuite.execution.current")
	}

	signals := &telemetrySet{
		tracer:       otel.Tracer(executionScope),
		launchCount:  launchCount,
		rejectCount:  rejectCount,
		finishCount:  finishCount,
		runDuration:  runDuration,
		stepDuration: stepDuration,
		stateGauge:   stateGauge,
	}

	if stateGauge != nil {
		registration, regErr := meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
			for status, total := range service.statusTallies() {
				observer.ObserveInt64(stateGauge, total, metric.WithAttributes(
					attribute.String("babelsuite.execution.status", status),
				))
			}
			return nil
		}, stateGauge)
		if regErr == nil {
			signals.registration = registration
		}
	}

	return signals
}

func (t *telemetrySet) shutdown() {
	if t == nil || t.registration == nil {
		return
	}
	_ = t.registration.Unregister()
}

func (s *Service) noteLaunch(ctx context.Context, suiteID string) {
	if s.signals == nil {
		return
	}
	s.signals.launchCount.Add(backgroundContext(ctx), 1, metric.WithAttributes(
		attribute.String("babelsuite.suite.id", cleanValue(suiteID)),
	))
}

func (s *Service) noteRejectedLaunch(ctx context.Context, suiteID, reason string) {
	if s.signals == nil {
		return
	}
	s.signals.rejectCount.Add(backgroundContext(ctx), 1, metric.WithAttributes(
		attribute.String("babelsuite.suite.id", cleanValue(suiteID)),
		attribute.String("babelsuite.execution.reason", cleanValue(reason)),
	))
}

func (s *Service) beginRunObservation(ctx context.Context, state *executionState) {
	if s.signals == nil || state == nil {
		return
	}

	spanCtx, span := s.signals.tracer.Start(backgroundContext(ctx), "suite.execution",
		trace.WithAttributes(executionAttributes(state.record)...),
	)
	state.monitor = &liveSpan{
		ctx:  trace.ContextWithSpan(context.Background(), span),
		span: span,
	}
	_ = spanCtx
}

func (s *Service) stepContext(executionID string) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil || item.monitor == nil || item.monitor.ctx == nil {
		return context.Background()
	}
	return item.monitor.ctx
}

func (s *Service) beginStepObservation(ctx context.Context, executionID string, suite *suites.Definition, profile string, node topologyNode) (context.Context, trace.Span, time.Time) {
	if s.signals == nil {
		return backgroundContext(ctx), nil, time.Now().UTC()
	}

	attrs := []attribute.KeyValue{
		attribute.String("babelsuite.execution.id", cleanValue(executionID)),
		attribute.String("babelsuite.suite.id", cleanValue(suite.ID)),
		attribute.String("babelsuite.suite.title", cleanValue(suite.Title)),
		attribute.String("babelsuite.execution.profile", cleanValue(profile)),
		attribute.String("babelsuite.step.id", cleanValue(node.ID)),
		attribute.String("babelsuite.step.kind", cleanValue(node.Kind)),
		attribute.Int("babelsuite.step.depends_on_count", len(node.DependsOn)),
	}

	stepCtx, span := s.signals.tracer.Start(backgroundContext(ctx), "suite.execution.step",
		trace.WithAttributes(attrs...),
	)
	return stepCtx, span, time.Now().UTC()
}

func (s *Service) finishStepObservation(ctx context.Context, span trace.Span, startedAt time.Time, executionID string, suite *suites.Definition, profile string, node topologyNode, err error) {
	if s.signals == nil {
		return
	}

	result := "ok"
	if err != nil {
		result = "error"
	}

	attributes := []attribute.KeyValue{
		attribute.String("babelsuite.execution.id", cleanValue(executionID)),
		attribute.String("babelsuite.suite.id", cleanValue(suite.ID)),
		attribute.String("babelsuite.execution.profile", cleanValue(profile)),
		attribute.String("babelsuite.step.kind", cleanValue(node.Kind)),
		attribute.String("babelsuite.step.result", result),
	}
	s.signals.stepDuration.Record(backgroundContext(ctx), time.Since(startedAt).Seconds(), metric.WithAttributes(attributes...))

	if span == nil {
		return
	}

	span.SetAttributes(attributes...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "step completed")
	}
	span.End()
}

func (s *Service) finishExecutionObservation(executionID string, err error) {
	if s.signals == nil {
		return
	}

	var (
		record  ExecutionRecord
		monitor *liveSpan
	)

	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil || item.monitor == nil {
		s.mu.Unlock()
		return
	}

	record = item.record
	record.Events = append([]ExecutionEvent{}, item.record.Events...)
	monitor = item.monitor
	item.monitor = nil
	s.mu.Unlock()

	attrs := append(executionAttributes(record),
		attribute.String("babelsuite.execution.status", normalizeStatus(record.Status)),
		attribute.Int("babelsuite.execution.event_count", len(record.Events)),
	)

	ctx := backgroundContext(monitor.ctx)
	s.signals.finishCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	s.signals.runDuration.Record(ctx, record.UpdatedAt.Sub(record.StartedAt).Seconds(), metric.WithAttributes(attrs...))

	if monitor.span == nil {
		return
	}

	monitor.span.SetAttributes(attrs...)
	if err != nil {
		monitor.span.RecordError(err)
		monitor.span.SetStatus(codes.Error, err.Error())
	} else {
		monitor.span.SetStatus(codes.Ok, record.Status)
	}
	monitor.span.End()
}

func (s *Service) statusTallies() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	counts := make(map[string]int64)
	for _, executionID := range s.order {
		item := s.executions[executionID]
		if item == nil {
			continue
		}
		counts[normalizeStatus(item.record.Status)]++
	}
	return counts
}

func executionAttributes(record ExecutionRecord) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("babelsuite.execution.id", cleanValue(record.ID)),
		attribute.String("babelsuite.suite.id", cleanValue(record.Suite.ID)),
		attribute.String("babelsuite.suite.title", cleanValue(record.Suite.Title)),
		attribute.String("babelsuite.execution.profile", cleanValue(record.Profile)),
		attribute.String("babelsuite.execution.trigger", cleanValue(record.Trigger)),
	}
}

func backgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeStatus(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
