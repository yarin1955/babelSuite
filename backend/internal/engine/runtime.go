package engine

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/babelsuite/babelsuite/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type StepLogger func(ctx context.Context, step *WorkflowStep, rc io.ReadCloser) error

type StepHooks struct {
	OnStart  func(step *WorkflowStep)
	OnFinish func(step *WorkflowStep, state *StepState, err error)
}

type Runtime struct {
	backend  Backend
	conf     *WorkflowConfig
	taskUUID string
	logger   StepLogger
	hooks    StepHooks

	uploads sync.WaitGroup
}

func NewRuntime(backend Backend, conf *WorkflowConfig, taskUUID string, logger StepLogger, hooks StepHooks) *Runtime {
	return &Runtime{
		backend:  backend,
		conf:     conf,
		taskUUID: taskUUID,
		logger:   logger,
		hooks:    hooks,
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	ctx, span := telemetry.Start(ctx, "babelsuite.runtime", "runtime.run", trace.WithAttributes(
		attribute.String("workflow.task_id", r.taskUUID),
		attribute.Int("workflow.stage_count", len(r.conf.Stages)),
	))
	defer span.End()

	defer func() {
		r.backend.DestroyWorkflow(ctx, r.conf, r.taskUUID)
		r.uploads.Wait()
	}()

	if err := r.backend.SetupWorkflow(ctx, r.conf, r.taskUUID); err != nil {
		span.RecordError(err)
		return fmt.Errorf("setup workflow: %w", err)
	}

	for index, stage := range r.conf.Stages {
		if err := r.execStage(ctx, index, stage); err != nil {
			span.RecordError(err)
			return err
		}
	}
	return nil
}

func (r *Runtime) execStage(ctx context.Context, index int, stage *WorkflowStage) error {
	ctx, span := telemetry.Start(ctx, "babelsuite.runtime", "runtime.stage", trace.WithAttributes(
		attribute.String("workflow.task_id", r.taskUUID),
		attribute.Int("workflow.stage_index", index),
		attribute.Int("workflow.step_count", len(stage.Steps)),
	))
	defer span.End()

	g, ctx := errgroup.WithContext(ctx)
	for _, step := range stage.Steps {
		step := step
		g.Go(func() error {
			return r.execStep(ctx, step)
		})
	}
	err := g.Wait()
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func (r *Runtime) execStep(ctx context.Context, step *WorkflowStep) error {
	ctx, span := telemetry.Start(ctx, "babelsuite.runtime", "runtime.step", trace.WithAttributes(
		attribute.String("workflow.task_id", r.taskUUID),
		attribute.String("workflow.step_id", step.UUID),
		attribute.String("workflow.step_name", step.Name),
		attribute.String("workflow.step_type", string(step.Type)),
		attribute.Bool("workflow.detached", step.Detached),
	))
	defer span.End()

	if err := r.backend.StartStep(ctx, step, r.taskUUID); err != nil {
		span.RecordError(err)
		r.finishHook(step, nil, err)
		return fmt.Errorf("start step %s: %w", step.Name, err)
	}
	r.startHook(step)

	rc, err := r.backend.TailStep(ctx, step, r.taskUUID)
	if err != nil {
		span.RecordError(err)
		r.finishHook(step, nil, err)
		return fmt.Errorf("tail step %s: %w", step.Name, err)
	}

	r.uploads.Add(1)
	if step.Detached {
		go func() {
			defer r.uploads.Done()
			if r.logger != nil {
				_ = r.logger(ctx, step, rc)
			}
			rc.Close()
		}()
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer r.uploads.Done()
		defer wg.Done()
		if r.logger != nil {
			_ = r.logger(ctx, step, rc)
		}
		rc.Close()
	}()
	wg.Wait()

	state, waitErr := r.backend.WaitStep(ctx, step, r.taskUUID)
	destroyErr := r.backend.DestroyStep(ctx, step, r.taskUUID)
	if waitErr != nil {
		span.RecordError(waitErr)
		r.finishHook(step, state, waitErr)
		return fmt.Errorf("wait step %s: %w", step.Name, waitErr)
	}
	if destroyErr != nil {
		span.RecordError(destroyErr)
		r.finishHook(step, state, destroyErr)
		return fmt.Errorf("destroy step %s: %w", step.Name, destroyErr)
	}
	if state.ExitCode != 0 {
		stepErr := fmt.Errorf("step %s: exit code %d", step.Name, state.ExitCode)
		span.RecordError(stepErr)
		r.finishHook(step, state, stepErr)
		return stepErr
	}

	r.finishHook(step, state, nil)
	return nil
}

func (r *Runtime) startHook(step *WorkflowStep) {
	if r.hooks.OnStart != nil {
		r.hooks.OnStart(step)
	}
}

func (r *Runtime) finishHook(step *WorkflowStep, state *StepState, err error) {
	if r.hooks.OnFinish != nil {
		r.hooks.OnFinish(step, state, err)
	}
}
