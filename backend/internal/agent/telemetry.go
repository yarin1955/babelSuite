package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const agentScope = "github.com/babelsuite/babelsuite/internal/agent"

type agentSignals struct {
	tracer        trace.Tracer
	leaseExpiries metric.Int64Counter
	jobsStarted   metric.Int64Counter
	jobsFinished  metric.Int64Counter
	registration  metric.Registration
}

func newAgentSignals() *agentSignals {
	meter := otel.Meter(agentScope)
	expiries, _ := meter.Int64Counter("babelsuite.agent.lease.expiries",
		metric.WithDescription("Number of jobs whose lease expired and were canceled"))
	started, _ := meter.Int64Counter("babelsuite.agent.jobs.started",
		metric.WithDescription("Number of jobs started by this agent"))
	finished, _ := meter.Int64Counter("babelsuite.agent.jobs.finished",
		metric.WithDescription("Number of jobs completed by this agent"))
	return &agentSignals{
		tracer:        otel.Tracer(agentScope),
		leaseExpiries: expiries,
		jobsStarted:   started,
		jobsFinished:  finished,
	}
}

var agentMetrics = newAgentSignals()

func registerCoordinatorGauge(c *Coordinator) {
	meter := otel.Meter(agentScope)
	gauge, err := meter.Int64ObservableGauge("babelsuite.agent.assignments",
		metric.WithDescription("Active coordinator assignments grouped by status"))
	if err != nil {
		return
	}
	reg, err := meter.RegisterCallback(func(_ context.Context, obs metric.Observer) error {
		for status, count := range c.statusTallies() {
			obs.ObserveInt64(gauge, count, metric.WithAttributes(
				attribute.String("assignment.status", status),
			))
		}
		return nil
	}, gauge)
	if err == nil {
		agentMetrics.registration = reg
	}
}

func jobAttributes(request StepRequest) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("agent.job_id", request.JobID),
		attribute.String("agent.execution_id", request.ExecutionID),
		attribute.String("agent.suite_id", request.SuiteID),
		attribute.String("agent.node_kind", request.Node.Kind),
	}
}
