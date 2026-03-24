package engine

import (
	"context"
	"fmt"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"
)

// StepLogger receives a step's log stream and is responsible for consuming it.
// It is called in a goroutine and must read rc until EOF.
type StepLogger func(step *WorkflowStep, rc io.ReadCloser) error

// Runtime executes a WorkflowConfig against a Backend, following the pattern
// from the design reference: SetupWorkflow → stages (parallel steps) → DestroyWorkflow.
type Runtime struct {
	backend  Backend
	conf     *WorkflowConfig
	taskUUID string
	logger   StepLogger

	// uploads tracks all logger goroutines so Run() waits for every log line
	// to be flushed before returning.
	uploads sync.WaitGroup
}

// NewRuntime creates a Runtime for the given workflow.
func NewRuntime(backend Backend, conf *WorkflowConfig, taskUUID string, logger StepLogger) *Runtime {
	return &Runtime{
		backend:  backend,
		conf:     conf,
		taskUUID: taskUUID,
		logger:   logger,
	}
}

// Run executes the workflow: setup → stages → cleanup.
// Stages run sequentially; steps within a stage run concurrently.
// DestroyWorkflow is always called on exit to clean up containers and networks.
// Run blocks until all logger goroutines have finished flushing logs.
func (r *Runtime) Run(ctx context.Context) error {
	defer func() {
		r.uploads.Wait() // drain all log goroutines before cleanup
		r.backend.DestroyWorkflow(ctx, r.conf, r.taskUUID)
	}()

	if err := r.backend.SetupWorkflow(ctx, r.conf, r.taskUUID); err != nil {
		return fmt.Errorf("setup workflow: %w", err)
	}

	for _, stage := range r.conf.Stages {
		if err := r.execStage(ctx, stage); err != nil {
			return err
		}
	}
	return nil
}

// execStage runs all steps in the stage concurrently and waits for all to finish.
func (r *Runtime) execStage(ctx context.Context, stage *WorkflowStage) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, step := range stage.Steps {
		step := step
		g.Go(func() error {
			return r.execStep(ctx, step)
		})
	}
	return g.Wait()
}

// execStep starts a container, streams its logs, and waits for completion.
// Detached steps (services) are started and left running; the log stream
// continues in a background goroutine until the container exits.
func (r *Runtime) execStep(ctx context.Context, step *WorkflowStep) error {
	if err := r.backend.StartStep(ctx, step, r.taskUUID); err != nil {
		return fmt.Errorf("start step %s: %w", step.Name, err)
	}

	rc, err := r.backend.TailStep(ctx, step, r.taskUUID)
	if err != nil {
		return fmt.Errorf("tail step %s: %w", step.Name, err)
	}

	r.uploads.Add(1)
	if step.Detached {
		// Service container: stream logs in background, don't wait for exit.
		go func() {
			defer r.uploads.Done()
			if r.logger != nil {
				_ = r.logger(step, rc)
			}
			rc.Close()
		}()
		return nil
	}

	// Non-detached: wait for all logs to be consumed, then check exit code.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer r.uploads.Done()
		defer wg.Done()
		if r.logger != nil {
			_ = r.logger(step, rc)
		}
		rc.Close()
	}()
	wg.Wait()

	state, err := r.backend.WaitStep(ctx, step, r.taskUUID)
	if err != nil {
		return fmt.Errorf("wait step %s: %w", step.Name, err)
	}
	if err := r.backend.DestroyStep(ctx, step, r.taskUUID); err != nil {
		return fmt.Errorf("destroy step %s: %w", step.Name, err)
	}
	if state.ExitCode != 0 {
		return fmt.Errorf("step %s: exit code %d", step.Name, state.ExitCode)
	}
	return nil
}
