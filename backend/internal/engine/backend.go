package engine

import (
	"context"
	"fmt"
	"io"
)

type RunnerOptions struct {
	EndpointURL           string
	InsecureSkipTLSVerify bool
	Username              string
	Password              string
	BearerToken           string
	TLSCAData             string
	TLSCertData           string
	TLSKeyData            string
}

// NewRunner creates a Runner by name. Currently supported: the default container backend.
// Add new cases here as additional backends are implemented.
func NewRunner(name string) (Runner, error) {
	return NewRunnerWithOptions(name, RunnerOptions{})
}

func NewRunnerWithOptions(name string, opts RunnerOptions) (Runner, error) {
	switch name {
	case "docker":
		return NewDockerWithOptions(opts)
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}

// BackendInfo is returned by Backend.Load and describes the execution environment.
type BackendInfo struct {
	Platform string // e.g. "linux/amd64"
}

// Backend orchestrates workflow execution across different runtimes.
// A single Backend instance handles multiple concurrent workflows.
// Each workflow is isolated by its taskUUID.
type Backend interface {
	// Name returns the backend identifier.
	Name() string

	// IsAvailable reports whether the backend is reachable and ready.
	IsAvailable(ctx context.Context) bool

	// Load initialises the backend and returns its platform information.
	Load(ctx context.Context) (*BackendInfo, error)

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
	Network string           // Network name created by SetupWorkflow
	Stages  []*WorkflowStage // Stages execute sequentially; steps within a stage run in parallel
}

// WorkflowStage is a set of steps that run concurrently.
type WorkflowStage struct {
	Steps []*WorkflowStep
}

// StepType identifies the role of a step within a workflow.
type StepType string

const (
	StepTypeClone    StepType = "clone"
	StepTypeService  StepType = "service"
	StepTypePlugin   StepType = "plugin"
	StepTypeCommands StepType = "commands"
	StepTypeCache    StepType = "cache"
)

// WorkflowStep describes a single container execution unit.
type WorkflowStep struct {
	UUID       string            // Unique identifier within the workflow
	Name       string            // Human-readable name (also used in container naming)
	Type       StepType          // Role of this step within the workflow
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

// Runner is implemented by backends that support the simple single-container
// execution model used by the agent (pull → start → tail → wait).
type Runner interface {
	Backend
	Pull(ctx context.Context, ref string) (<-chan string, error)
	Start(ctx context.Context, cfg RunConfig) (string, error)
	Tail(ctx context.Context, containerID string) (io.ReadCloser, error)
	Wait(ctx context.Context, containerID string) (int, error)
	Stop(ctx context.Context, containerID string)
	CreateNetwork(ctx context.Context, name string) (string, error)
	RemoveNetwork(ctx context.Context, networkID string)
	Close() error
}
