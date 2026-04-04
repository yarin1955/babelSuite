package execution

import (
	"context"
	"errors"
	"regexp"
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
	ErrExecutionNotFound  = errors.New("execution not found")
	ErrBackendNotFound    = errors.New("backend not found")
	ErrBackendUnavailable = errors.New("backend unavailable")
)

var topologyPattern = regexp.MustCompile(`^([a-zA-Z_][\w]*)\s*=\s*(container|mock|script|load|scenario)\(\s*name\s*=\s*"([^"]+)"(?:,\s*after=\[([^\]]*)\])?.*\)$`)

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
	BackendID string           `json:"backendId"`
	Backend   string           `json:"backend"`
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
	Backend string `json:"backend,omitempty"`
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

type executionState struct {
	record    ExecutionRecord
	total     int
	completed int
	monitor   *liveSpan
}
