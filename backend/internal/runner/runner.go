package runner

import (
	"context"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type StepNode struct {
	ID        string
	Name      string
	Kind      string
	DependsOn []string
}

type StepSpec struct {
	ExecutionID string
	SuiteID     string
	SuiteTitle  string
	Profile     string
	Node        StepNode
}

type Executor interface {
	Run(ctx context.Context, step StepSpec, emit func(logstream.Line)) error
}
