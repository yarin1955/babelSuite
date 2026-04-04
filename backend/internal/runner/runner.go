package runner

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type StepNode struct {
	ID        string
	Name      string
	Kind      string
	DependsOn []string
}

type StepSpec struct {
	ExecutionID     string
	SuiteID         string
	SuiteTitle      string
	SuiteRepository string
	Profile         string
	Trigger         string
	BackendID       string
	BackendLabel    string
	BackendKind     string
	StepIndex       int
	TotalSteps      int
	LeaseTTL        time.Duration
	Node            StepNode
}

type Executor interface {
	Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error
}

type Backend interface {
	Executor
	ID() string
	Label() string
	Kind() string
	IsAvailable(ctx context.Context) bool
}

type BackendConfig struct {
	ID    string
	Label string
	Kind  string
}
