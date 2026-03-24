package engine

import (
	"context"
	"io"
)

// Backend orchestrates workflow execution across different runtimes (Docker, etc.).
// A single Backend instance handles multiple concurrent workflows.
// Each workflow is isolated by its taskUUID.
type Backend interface {
	// SetupWorkflow creates the isolated environment for a workflow (network, volume).
	SetupWorkflow(ctx context.Context, conf *WorkflowConfig, taskUUID string) error

	// StartStep creates and starts a container for the given step.
	StartStep(ctx context.Context, step *WorkflowStep, taskUUID string) error

	// TailStep returns an io.ReadCloser that streams the step's combined stdout+stderr.
	TailStep(ctx context.Context, step *WorkflowStep, taskUUID string) (io.ReadCloser, error)

	// WaitStep blocks until the step's container exits and returns its state.
	WaitStep(ctx context.Context, step *WorkflowStep, taskUUID string) (*StepState, error)

	// DestroyStep stops and removes the step's container.
	DestroyStep(ctx context.Context, step *WorkflowStep, taskUUID string) error

	// DestroyWorkflow tears down all workflow resources (containers, network).
	DestroyWorkflow(ctx context.Context, conf *WorkflowConfig, taskUUID string) error
}

// WorkflowConfig describes a multi-stage workflow to execute.
type WorkflowConfig struct {
	Network string           // Docker network name (created by SetupWorkflow)
	Stages  []*WorkflowStage // Stages execute sequentially; steps within a stage run in parallel
}

// WorkflowStage is a set of steps that run concurrently.
type WorkflowStage struct {
	Steps []*WorkflowStep
}

// WorkflowStep describes a single container execution unit.
type WorkflowStep struct {
	UUID       string            // Unique identifier within the workflow
	Name       string            // Human-readable name (also used in container naming)
	Image      string            // OCI image reference
	Pull       bool              // Always pull the image before starting
	Detached   bool              // Service container: start and keep running; don't wait for exit
	Entrypoint []string          // Override image entrypoint
	Cmd        []string          // Command / args passed to the container
	Env        map[string]string // Environment variables
}

// StepState is the result of a completed step.
type StepState struct {
	ExitCode  int
	Exited    bool
	OOMKilled bool
}
