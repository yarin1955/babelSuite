package runner

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type StepNode struct {
	ID        string
	Name      string
	Kind      string
	Variant   string
	Image     string
	DependsOn []string
}

type ArtifactExport struct {
	Path   string
	Name   string
	On     string
	Format string
}

type StepSpec struct {
	ExecutionID      string
	SuiteID          string
	SuiteTitle       string
	SuiteRepository  string
	Profile          string
	RuntimeProfile   string
	Env              map[string]string
	Headers          map[string]string
	Trigger          string
	BackendID        string
	BackendLabel     string
	BackendKind      string
	SourceSuiteID    string
	SourceSuiteTitle string
	SourceRepository string
	SourceVersion    string
	ResolvedRef      string
	Digest           string
	DependencyAlias  string
	StepIndex        int
	TotalSteps       int
	LeaseTTL         time.Duration
	Load             *suites.LoadSpec
	Evaluation       *suites.StepEvaluation
	OnFailure        []string
	ArtifactExports  []ArtifactExport
	Node             StepNode
	// GatewayURL is the address of the APISIX sidecar for this execution.
	// It is set automatically when the suite has a gateway (mock) node.
	GatewayURL string
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
