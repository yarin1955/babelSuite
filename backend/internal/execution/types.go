package execution

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/queue"
	"github.com/babelsuite/babelsuite/internal/runner"
	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSuiteNotFound     = errors.New("suite not found")
	ErrProfileNotFound   = errors.New("profile not found")
	ErrExecutionNotFound = errors.New("execution not found")
	ErrInvalidTopology   = errors.New("invalid suite topology")
)

var (
	topologyAssignmentPattern = regexp.MustCompile(`^([a-zA-Z_][\w]*)\s*=\s*([a-zA-Z_][\w]*(?:\.[a-zA-Z_][\w]*)?)\((.*)\)$`)
	topologyNamePattern       = regexp.MustCompile(`(?:^|,)\s*(?:name|name_or_id|id)\s*=\s*"([^"]+)"`)
	topologyAfterPattern      = regexp.MustCompile(`(?:^|,)\s*after\s*=\s*\[([^\]]*)\]`)
)

type ProfileOption struct {
	FileName    string `json:"fileName"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type LaunchSuite struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Repository  string          `json:"repository"`
	Description string          `json:"description"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	Profiles    []ProfileOption `json:"profiles"`
}

type ExecutionSummary struct {
	ID         string    `json:"id"`
	SuiteID    string    `json:"suiteId"`
	SuiteTitle string    `json:"suiteTitle"`
	Profile    string    `json:"profile"`
	Trigger    string    `json:"trigger"`
	Status     string    `json:"status"`
	Duration   string    `json:"duration"`
	StartedAt  time.Time `json:"startedAt"`
}

type ExecutionSuite struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Repository  string               `json:"repository"`
	SuiteStar   string               `json:"suiteStar"`
	Profiles    []ProfileOption      `json:"profiles"`
	Folders     []suites.FolderEntry `json:"folders"`
	SourceFiles []suites.SourceFile  `json:"sourceFiles"`
	APISurfaces []suites.APISurface  `json:"apiSurfaces"`
}

type ExecutionEvent struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	Level     string `json:"level"`
}

type ExecutionRecord struct {
	ID        string           `json:"id"`
	Suite     ExecutionSuite   `json:"suite"`
	Profile   string           `json:"profile"`
	Trigger   string           `json:"trigger"`
	Status    string           `json:"status"`
	Duration  string           `json:"duration"`
	StartedAt time.Time        `json:"startedAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Author    string           `json:"author"`
	Commit    string           `json:"commit"`
	Branch    string           `json:"branch"`
	Message   string           `json:"message"`
	Events    []ExecutionEvent `json:"events"`
}

type CreateRequest struct {
	SuiteID string `json:"suiteId"`
	Profile string `json:"profile"`
}

type StepSnapshot struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	DependsOn []string `json:"dependsOn"`
}

type Snapshot struct {
	ID            string         `json:"id"`
	SuiteID       string         `json:"suiteId"`
	SuiteTitle    string         `json:"suiteTitle"`
	Profile       string         `json:"profile"`
	Trigger       string         `json:"trigger"`
	Status        string         `json:"status"`
	Duration      string         `json:"duration"`
	StartedAt     time.Time      `json:"startedAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	TotalSteps    int            `json:"totalSteps"`
	RunningSteps  int            `json:"runningSteps"`
	HealthySteps  int            `json:"healthySteps"`
	FailedSteps   int            `json:"failedSteps"`
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

	suiteSource suiteSource
	observers   []Observer
	logs        *logstream.Hub
	runner      runner.Executor
	signals     *telemetrySet

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

type suiteRuntimeMeta struct {
	Author         string
	Branch         string
	Message        string
	FailureTarget  string
	DefaultTrigger string
}

type topologyNode struct {
	ID        string
	Name      string
	Kind      string
	DependsOn []string
	Level     int
	Order     int
}

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
	record    ExecutionRecord
	total     int
	completed int
	monitor   *liveSpan
}
