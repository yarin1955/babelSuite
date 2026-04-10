package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/queue"
	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSuiteNotFound      = errors.New("suite not found")
	ErrProfileNotFound    = errors.New("profile not found")
	ErrProfileRuntime     = errors.New("profile runtime error")
	ErrExecutionNotFound  = errors.New("execution not found")
	ErrBackendNotFound    = errors.New("backend not found")
	ErrBackendUnavailable = errors.New("backend unavailable")
	ErrInvalidTopology    = errors.New("invalid suite topology")
)

type ProfileOption struct {
	FileName    string `json:"fileName"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type BackendOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
	Available   bool   `json:"available"`
}

type LaunchSuite struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Repository  string          `json:"repository"`
	Description string          `json:"description"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	Profiles    []ProfileOption `json:"profiles"`
	Backends    []BackendOption `json:"backends"`
}

type ExecutionSummary struct {
	ID         string    `json:"id"`
	SuiteID    string    `json:"suiteId"`
	SuiteTitle string    `json:"suiteTitle"`
	Profile    string    `json:"profile"`
	BackendID  string    `json:"backendId"`
	Backend    string    `json:"backend"`
	Trigger    string    `json:"trigger"`
	Status     string    `json:"status"`
	Duration   string    `json:"duration"`
	StartedAt  time.Time `json:"startedAt"`
}

type ExecutionSuite struct {
	ID                   string                      `json:"id"`
	Title                string                      `json:"title"`
	Repository           string                      `json:"repository"`
	SuiteStar            string                      `json:"suiteStar"`
	Profiles             []ProfileOption             `json:"profiles"`
	Folders              []suites.FolderEntry        `json:"folders"`
	SourceFiles          []suites.SourceFile         `json:"sourceFiles"`
	Topology             []suites.TopologyNode       `json:"topology,omitempty"`
	ResolvedDependencies []suites.ResolvedDependency `json:"resolvedDependencies,omitempty"`
	APISurfaces          []suites.APISurface         `json:"apiSurfaces"`
}

type ExecutionEvent struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	Level     string `json:"level"`
}

type ExecutionTestSummary struct {
	Total           int     `json:"total"`
	Passed          int     `json:"passed"`
	Failures        int     `json:"failures"`
	Errors          int     `json:"errors"`
	Skipped         int     `json:"skipped"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
}

type ExecutionCoverageSummary struct {
	LineRate        float64 `json:"lineRate,omitempty"`
	BranchRate      float64 `json:"branchRate,omitempty"`
	LinesCovered    int     `json:"linesCovered,omitempty"`
	LinesValid      int     `json:"linesValid,omitempty"`
	BranchesCovered int     `json:"branchesCovered,omitempty"`
	BranchesValid   int     `json:"branchesValid,omitempty"`
}

type ExecutionArtifact struct {
	ID              string                    `json:"id"`
	StepID          string                    `json:"stepId"`
	StepName        string                    `json:"stepName"`
	Path            string                    `json:"path"`
	Name            string                    `json:"name"`
	On              string                    `json:"on,omitempty"`
	Format          string                    `json:"format,omitempty"`
	Content         string                    `json:"content,omitempty"`
	TestSummary     *ExecutionTestSummary     `json:"testSummary,omitempty"`
	CoverageSummary *ExecutionCoverageSummary `json:"coverageSummary,omitempty"`
}

type ExecutionRecord struct {
	ID        string              `json:"id"`
	Suite     ExecutionSuite      `json:"suite"`
	Profile   string              `json:"profile"`
	BackendID string              `json:"backendId"`
	Backend   string              `json:"backend"`
	Trigger   string              `json:"trigger"`
	Status    string              `json:"status"`
	Duration  string              `json:"duration"`
	StartedAt time.Time           `json:"startedAt"`
	UpdatedAt time.Time           `json:"updatedAt"`
	Author    string              `json:"author"`
	Commit    string              `json:"commit"`
	Branch    string              `json:"branch"`
	Message   string              `json:"message"`
	Events    []ExecutionEvent    `json:"events"`
	Artifacts []ExecutionArtifact `json:"artifacts,omitempty"`
}

type CreateRequest struct {
	SuiteID string `json:"suiteId"`
	Profile string `json:"profile"`
	Backend string `json:"backend,omitempty"`
}

type StepSnapshot struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	DependsOn []string `json:"dependsOn"`
	Level     int      `json:"level"`
}

type Snapshot struct {
	ID            string         `json:"id"`
	SuiteID       string         `json:"suiteId"`
	SuiteTitle    string         `json:"suiteTitle"`
	Profile       string         `json:"profile"`
	BackendID     string         `json:"backendId"`
	Backend       string         `json:"backend"`
	Trigger       string         `json:"trigger"`
	Status        string         `json:"status"`
	Duration      string         `json:"duration"`
	StartedAt     time.Time      `json:"startedAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	TotalSteps    int            `json:"totalSteps"`
	RunningSteps  int            `json:"runningSteps"`
	HealthySteps  int            `json:"healthySteps"`
	FailedSteps   int            `json:"failedSteps"`
	SkippedSteps  int            `json:"skippedSteps,omitempty"`
	PendingSteps  int            `json:"pendingSteps"`
	ProgressRatio float64        `json:"progressRatio"`
	Steps         []StepSnapshot `json:"steps"`
}

type Observer interface {
	SyncExecution(snapshot Snapshot)
}

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	queue  *queue.Memory

	suiteSource    suiteSource
	platformSource platformSource
	mockResetter   mockResetter
	registry       *agent.Registry
	coordinator    *agent.Coordinator
	runtimeStore   RuntimeStore
	runtimeCache   executionCache
	runtimeTTL     time.Duration
	observers      []Observer
	logs           *logstream.Hub
	signals        *telemetrySet

	mu         sync.Mutex
	suiteMeta  map[string]suiteRuntimeMeta
	executions map[string]*executionState
	subs       map[string]map[chan StreamEvent]struct{}
	order      []string
}

type suiteSource interface {
	List() []suites.Definition
	Get(id string) (*suites.Definition, error)
}

type platformSource interface {
	Load() (*platform.PlatformSettings, error)
}

type mockResetter interface {
	ResetSuiteState(ctx context.Context, suiteID string) error
}

type suiteRuntimeMeta struct {
	Author         string
	Branch         string
	Message        string
	FailureTarget  string
	DefaultTrigger string
}

type topologyNode = suites.TopologyNode

type topologyDuplicateNodeError struct {
	Node string
}

func (e *topologyDuplicateNodeError) Error() string {
	return "invalid suite topology: duplicate step " + `"` + e.Node + `"`
}

func (e *topologyDuplicateNodeError) Is(target error) bool {
	return target == ErrInvalidTopology
}

type topologyMissingDependencyError struct {
	Node       string
	Dependency string
}

func (e *topologyMissingDependencyError) Error() string {
	return "invalid suite topology: " + `"` + e.Node + `"` + " depends on missing step " + `"` + e.Dependency + `"`
}

func (e *topologyMissingDependencyError) Is(target error) bool {
	return target == ErrInvalidTopology
}

type topologyCycleError struct {
	Path []string
}

func (e *topologyCycleError) Error() string {
	return "invalid suite topology: dependency cycle detected: " + strings.Join(e.Path, " -> ")
}

func (e *topologyCycleError) Is(target error) bool {
	return target == ErrInvalidTopology
}

type executionState struct {
	record      ExecutionRecord
	runtime     executionRuntimeOverlay
	total       int
	completed   int
	stepStatus  map[string]string
	terminalErr error
	monitor     *liveSpan
}

type executionRuntimeOverlay struct {
	Env       map[string]string
	Services  map[string]map[string]string
	SecretEnv map[string]string
}
