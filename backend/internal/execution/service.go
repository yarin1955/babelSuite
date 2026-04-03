package execution

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/mocking"
	"github.com/babelsuite/babelsuite/internal/queue"
	"github.com/babelsuite/babelsuite/internal/runner"
	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSuiteNotFound     = errors.New("suite not found")
	ErrProfileNotFound   = errors.New("profile not found")
	ErrExecutionNotFound = errors.New("execution not found")
)

var topologyPattern = regexp.MustCompile(`^([a-zA-Z_][\w]*)\s*=\s*(container|mock|script|load|scenario)\(\s*name\s*=\s*"([^"]+)"(?:,\s*after=\[([^\]]*)\])?.*\)$`)

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
}

type executionState struct {
	record    ExecutionRecord
	total     int
	completed int
	monitor   *liveSpan
}

func NewService(source suiteSource, observers ...Observer) *Service {
	if source == nil {
		source = suites.NewService()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		ctx:         ctx,
		cancel:      cancel,
		queue:       queue.NewMemory(ctx, 3),
		suiteSource: source,
		observers:   observers,
		logs:        logstream.NewHub(),
		runner:      runner.NewLocal(),
		suiteMeta:   seedExecutionMetadata(),
		executions:  make(map[string]*executionState),
		subs:        make(map[string]map[chan StreamEvent]struct{}),
	}
	s.signals = newTelemetrySet(s)
	s.seedHistory()
	return s
}

func (s *Service) Close() {
	if s.signals != nil {
		s.signals.shutdown()
	}
	s.cancel()
	s.queue.Close()
}

func (s *Service) ListLaunchSuites() []LaunchSuite {
	result := make([]LaunchSuite, 0, len(s.suiteSource.List()))
	for _, suite := range s.suiteSource.List() {
		result = append(result, LaunchSuite{
			ID:          suite.ID,
			Title:       suite.Title,
			Repository:  suite.Repository,
			Description: suite.Description,
			Provider:    suite.Provider,
			Status:      suite.Status,
			Profiles:    toExecutionProfiles(suite.Profiles),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Title < result[j].Title
	})
	return result
}

func (s *Service) ListExecutions() []ExecutionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]ExecutionSummary, 0, len(s.order))
	for i := len(s.order) - 1; i >= 0; i-- {
		item := s.executions[s.order[i]]
		if item == nil {
			continue
		}
		result = append(result, s.summaryLocked(item))
	}
	return result
}

func (s *Service) GetExecution(executionID string) (*ExecutionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return nil, ErrExecutionNotFound
	}

	record := item.record
	record.Duration = s.durationLocked(item)
	record.Events = append([]ExecutionEvent{}, item.record.Events...)
	record.Suite = cloneExecutionSuite(item.record.Suite)
	return &record, nil
}

func (s *Service) CreateExecution(ctx context.Context, request CreateRequest) (*ExecutionSummary, error) {
	suiteID := strings.TrimSpace(request.SuiteID)
	s.noteLaunch(ctx, suiteID)

	suite, err := s.suiteSource.Get(suiteID)
	if err != nil {
		s.noteRejectedLaunch(ctx, suiteID, "suite_not_found")
		return nil, ErrSuiteNotFound
	}
	meta := s.suiteMeta[suite.ID]

	profile := strings.TrimSpace(request.Profile)
	if profile == "" {
		profile = defaultProfile(suite.Profiles)
	}
	if !suiteHasProfile(suite.Profiles, profile) {
		s.noteRejectedLaunch(ctx, suite.ID, "profile_not_found")
		return nil, ErrProfileNotFound
	}

	executionID := "run-" + uuid.NewString()[:8]
	startedAt := time.Now().UTC()
	state := &executionState{
		record: ExecutionRecord{
			ID:        executionID,
			Suite:     buildExecutionSuite(*suite),
			Profile:   profile,
			Trigger:   firstNonEmpty(meta.DefaultTrigger, "Manual"),
			Status:    "Booting",
			StartedAt: startedAt,
			UpdatedAt: startedAt,
			Author:    meta.Author,
			Commit:    buildCommitHash(suite.ID, executionID),
			Branch:    meta.Branch,
			Message:   meta.Message,
			Events:    []ExecutionEvent{},
		},
	}

	topology := parseSuiteTopology(suite.SuiteStar)
	state.total = len(topology)

	s.mu.Lock()
	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.mu.Unlock()
	s.logs.Open(executionID)
	s.beginRunObservation(ctx, state)

	tasks := make([]queue.Task, 0, len(topology))
	taskIDs := make(map[string]string, len(topology))
	for _, node := range topology {
		taskIDs[node.ID] = executionID + ":" + node.ID
	}
	for _, node := range topology {
		dependencies := make([]string, 0, len(node.DependsOn))
		for _, dependency := range node.DependsOn {
			if dependencyID := taskIDs[dependency]; dependencyID != "" {
				dependencies = append(dependencies, dependencyID)
			}
		}

		node := node
		tasks = append(tasks, queue.Task{
			ID:           taskIDs[node.ID],
			Group:        executionID,
			Name:         node.Name,
			Dependencies: dependencies,
			LeaseTTL:     8 * time.Second,
			Run: func(ctx context.Context) error {
				return s.runNode(ctx, executionID, suite, profile, node)
			},
		})
	}

	if err := s.queue.Enqueue(tasks); err != nil {
		s.noteRejectedLaunch(ctx, suite.ID, "enqueue_failed")
		s.mu.Lock()
		if item := s.executions[executionID]; item != nil {
			item.record.Status = "Failed"
			item.record.UpdatedAt = time.Now().UTC()
		}
		s.mu.Unlock()
		s.finishExecutionObservation(executionID, err)

		s.mu.Lock()
		delete(s.executions, executionID)
		s.order = filterOut(s.order, executionID)
		s.mu.Unlock()
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	summary := s.summaryLocked(state)
	go s.syncObservers(executionID)
	return &summary, nil
}

func (s *Service) runNode(ctx context.Context, executionID string, suite *suites.Definition, profile string, node topologyNode) error {
	stepCtx, stepSpan, stepStartedAt := s.beginStepObservation(s.stepContext(executionID), executionID, suite, profile, node)

	s.appendEvent(executionID, ExecutionEvent{
		ID:        node.ID + "-start",
		Source:    node.ID,
		Timestamp: s.nextTimestamp(executionID),
		Text:      buildStartMessage(node, suite, profile),
		Status:    "running",
		Level:     "info",
	})

	err := s.runner.Run(stepCtx, runner.StepSpec{
		ExecutionID: executionID,
		SuiteID:     suite.ID,
		SuiteTitle:  suite.Title,
		Profile:     profile,
		Node: runner.StepNode{
			ID:        node.ID,
			Name:      node.Name,
			Kind:      node.Kind,
			DependsOn: append([]string{}, node.DependsOn...),
		},
	}, func(line logstream.Line) {
		s.appendRunnerLog(executionID, node.ID, line)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.failExecution(executionID, node.ID, fmt.Sprintf("[%s] Execution canceled before the node became healthy.", node.Name))
			s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, context.Canceled)
			s.finishExecutionObservation(executionID, context.Canceled)
			return context.Canceled
		}
		s.failExecution(executionID, node.ID, fmt.Sprintf("[%s] Runner failed: %v", node.Name, err))
		s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, err)
		s.finishExecutionObservation(executionID, err)
		return err
	}

	finished := s.markNodeComplete(executionID)
	s.appendEvent(executionID, ExecutionEvent{
		ID:        node.ID + "-healthy",
		Source:    node.ID,
		Timestamp: s.nextTimestamp(executionID),
		Text:      buildHealthyMessage(node, suite, profile),
		Status:    "healthy",
		Level:     "info",
	})
	s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, nil)
	if finished {
		s.finishExecutionObservation(executionID, nil)
	}

	return nil
}

func (s *Service) appendEvent(executionID string, event ExecutionEvent) {
	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return
	}

	item.record.Events = append(item.record.Events, event)
	item.record.UpdatedAt = time.Now().UTC()
	streamEvent := StreamEvent{
		ID:              len(item.record.Events),
		ExecutionID:     executionID,
		ExecutionStatus: item.record.Status,
		Duration:        s.durationLocked(item),
		UpdatedAt:       item.record.UpdatedAt,
		Event:           event,
	}
	subscribers := collectSubscribers(s.subs[executionID])
	s.mu.Unlock()

	s.publish(streamEvent, subscribers)
	s.appendLog(executionID, event)
	s.syncObservers(executionID)
}

func (s *Service) nextTimestamp(executionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return "00:00"
	}

	second := len(item.record.Events)
	return fmt.Sprintf("%02d:%02d", second/60, second%60)
}

func (s *Service) markNodeComplete(executionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return false
	}

	item.completed++
	item.record.UpdatedAt = time.Now().UTC()
	if item.completed >= item.total {
		item.record.Status = "Healthy"
		return true
	}
	return false
}

func (s *Service) failExecution(executionID, source, message string) {
	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return
	}
	if item.record.Status == "Failed" {
		s.mu.Unlock()
		return
	}

	item.record.Status = "Failed"
	item.record.UpdatedAt = time.Now().UTC()
	event := ExecutionEvent{
		ID:        source + "-failed",
		Source:    source,
		Timestamp: fmt.Sprintf("%02d:%02d", len(item.record.Events)/60, len(item.record.Events)%60),
		Text:      message,
		Status:    "failed",
		Level:     "error",
	}
	item.record.Events = append(item.record.Events, event)
	streamEvent := StreamEvent{
		ID:              len(item.record.Events),
		ExecutionID:     executionID,
		ExecutionStatus: item.record.Status,
		Duration:        s.durationLocked(item),
		UpdatedAt:       item.record.UpdatedAt,
		Event:           event,
	}
	subscribers := collectSubscribers(s.subs[executionID])
	s.mu.Unlock()

	s.queue.CancelGroup(executionID)
	s.publish(streamEvent, subscribers)
	s.appendLog(executionID, event)
	s.syncObservers(executionID)
}

func (s *Service) seedHistory() {
	s.seedHistoricalExecution("run-1043", "payment-suite", "staging.yaml", "Manual", "Healthy", 2*time.Minute)
	s.seedHistoricalExecution("run-1042", "fleet-control-room", "perf.yaml", "CI", "Healthy", 7*time.Minute)
	s.seedHistoricalExecution("run-1041", "identity-broker", "ci.yaml", "CI", "Failed", 24*time.Minute)
}

func (s *Service) seedHistoricalExecution(executionID, suiteID, profile, trigger, status string, startedAgo time.Duration) {
	suite, err := s.suiteSource.Get(suiteID)
	if err != nil {
		return
	}
	meta := s.suiteMeta[suiteID]

	startedAt := time.Now().UTC().Add(-startedAgo)
	topology := parseSuiteTopology(suite.SuiteStar)
	events := buildHistoricalEvents(suite, topology, status, profile, meta)

	state := &executionState{
		record: ExecutionRecord{
			ID:        executionID,
			Suite:     buildExecutionSuite(*suite),
			Profile:   profile,
			Trigger:   trigger,
			Status:    status,
			StartedAt: startedAt,
			UpdatedAt: startedAt.Add(time.Duration(len(events)) * 2 * time.Second),
			Author:    meta.Author,
			Commit:    buildCommitHash(suite.ID, executionID),
			Branch:    meta.Branch,
			Message:   meta.Message,
			Events:    events,
		},
		total:     len(topology),
		completed: len(topology),
	}

	if status == "Failed" {
		state.completed = max(len(topology)-1, 0)
	}

	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.logs.Open(executionID)
	for _, event := range events {
		s.appendLog(executionID, event)
	}
	go s.syncObservers(executionID)
}

func (s *Service) summaryLocked(item *executionState) ExecutionSummary {
	return ExecutionSummary{
		ID:         item.record.ID,
		SuiteID:    item.record.Suite.ID,
		SuiteTitle: item.record.Suite.Title,
		Profile:    item.record.Profile,
		Trigger:    item.record.Trigger,
		Status:     item.record.Status,
		Duration:   s.durationLocked(item),
		StartedAt:  item.record.StartedAt,
	}
}

func (s *Service) durationLocked(item *executionState) string {
	end := item.record.UpdatedAt
	if end.Before(item.record.StartedAt) {
		end = item.record.StartedAt
	}
	return formatDuration(end.Sub(item.record.StartedAt))
}

func suiteHasProfile(profiles []suites.ProfileOption, fileName string) bool {
	for _, profile := range profiles {
		if profile.FileName == fileName {
			return true
		}
	}
	return false
}

func defaultProfile(profiles []suites.ProfileOption) string {
	for _, profile := range profiles {
		if profile.Default {
			return profile.FileName
		}
	}
	if len(profiles) > 0 {
		return profiles[0].FileName
	}
	return ""
}

func parseSuiteTopology(suiteStar string) []topologyNode {
	nodes := make([]topologyNode, 0)
	for _, rawLine := range strings.Split(suiteStar, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		match := topologyPattern.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}

		dependsOn := []string{}
		if match[4] != "" {
			for _, dependency := range strings.Split(match[4], ",") {
				dependency = strings.TrimSpace(strings.ReplaceAll(dependency, "\"", ""))
				if dependency != "" {
					dependsOn = append(dependsOn, dependency)
				}
			}
		}

		nodes = append(nodes, topologyNode{
			ID:        match[3],
			Name:      match[3],
			Kind:      match[2],
			DependsOn: dependsOn,
		})
	}

	byID := make(map[string]*topologyNode, len(nodes))
	for i := range nodes {
		byID[nodes[i].ID] = &nodes[i]
	}

	levelCache := make(map[string]int, len(nodes))
	var resolveLevel func(string) int
	resolveLevel = func(id string) int {
		if level, ok := levelCache[id]; ok {
			return level
		}

		node := byID[id]
		if node == nil || len(node.DependsOn) == 0 {
			levelCache[id] = 0
			return 0
		}

		level := 0
		for _, dependency := range node.DependsOn {
			level = max(level, resolveLevel(dependency)+1)
		}
		levelCache[id] = level
		return level
	}

	for i := range nodes {
		nodes[i].Level = resolveLevel(nodes[i].ID)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Level != nodes[j].Level {
			return nodes[i].Level < nodes[j].Level
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}

func buildHistoricalEvents(suite *suites.Definition, topology []topologyNode, status, profile string, meta suiteRuntimeMeta) []ExecutionEvent {
	events := make([]ExecutionEvent, 0, len(topology)*2)
	for _, node := range topology {
		events = append(events, ExecutionEvent{
			ID:        node.ID + "-start",
			Source:    node.ID,
			Timestamp: fmt.Sprintf("%02d:%02d", len(events)/60, len(events)%60),
			Text:      buildStartMessage(node, suite, profile),
			Status:    "running",
			Level:     "info",
		})

		if status == "Failed" && node.ID == meta.FailureTarget {
			events = append(events, ExecutionEvent{
				ID:        node.ID + "-failed",
				Source:    node.ID,
				Timestamp: fmt.Sprintf("%02d:%02d", len(events)/60, len(events)%60),
				Text:      buildFailureMessage(node, suite),
				Status:    "failed",
				Level:     "error",
			})
			break
		}

		events = append(events, ExecutionEvent{
			ID:        node.ID + "-healthy",
			Source:    node.ID,
			Timestamp: fmt.Sprintf("%02d:%02d", len(events)/60, len(events)%60),
			Text:      buildHealthyMessage(node, suite, profile),
			Status:    "healthy",
			Level:     "info",
		})
	}
	return events
}

func (s *Service) syncObservers(executionID string) {
	if len(s.observers) == 0 {
		return
	}

	snapshot, ok := s.snapshotExecution(executionID)
	if !ok {
		return
	}

	for _, observer := range s.observers {
		if observer == nil {
			continue
		}
		observer.SyncExecution(snapshot)
	}
}

func (s *Service) appendLog(executionID string, event ExecutionEvent) {
	s.logs.Append(executionID, logstream.Line{
		Source:    event.Source,
		Timestamp: event.Timestamp,
		Level:     event.Level,
		Text:      event.Text,
	})
}

func (s *Service) appendRunnerLog(executionID, source string, line logstream.Line) {
	timestamp := line.Timestamp
	if strings.TrimSpace(timestamp) == "" {
		timestamp = s.nextTimestamp(executionID)
	}

	s.logs.Append(executionID, logstream.Line{
		Source:    firstNonEmpty(line.Source, source),
		Timestamp: timestamp,
		Level:     firstNonEmpty(line.Level, "info"),
		Text:      line.Text,
	})
}

func (s *Service) snapshotExecution(executionID string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return Snapshot{}, false
	}

	topology := parseSuiteTopology(item.record.Suite.SuiteStar)
	statuses := make(map[string]string, len(topology))
	for _, node := range topology {
		statuses[node.ID] = "pending"
	}
	for _, event := range item.record.Events {
		statuses[event.Source] = event.Status
	}

	steps := make([]StepSnapshot, 0, len(topology))
	runningSteps := 0
	healthySteps := 0
	failedSteps := 0
	pendingSteps := 0

	for _, node := range topology {
		status := statuses[node.ID]
		switch status {
		case "running":
			runningSteps++
		case "healthy":
			healthySteps++
		case "failed":
			failedSteps++
		default:
			pendingSteps++
		}

		steps = append(steps, StepSnapshot{
			ID:        node.ID,
			Name:      node.Name,
			Kind:      node.Kind,
			Status:    status,
			DependsOn: append([]string{}, node.DependsOn...),
		})
	}

	progressRatio := 0.0
	if len(steps) > 0 {
		progressRatio = float64(healthySteps+runningSteps+failedSteps) / float64(len(steps))
	}

	return Snapshot{
		ID:            item.record.ID,
		SuiteID:       item.record.Suite.ID,
		SuiteTitle:    item.record.Suite.Title,
		Profile:       item.record.Profile,
		Trigger:       item.record.Trigger,
		Status:        item.record.Status,
		Duration:      s.durationLocked(item),
		StartedAt:     item.record.StartedAt,
		UpdatedAt:     item.record.UpdatedAt,
		TotalSteps:    len(steps),
		RunningSteps:  runningSteps,
		HealthySteps:  healthySteps,
		FailedSteps:   failedSteps,
		PendingSteps:  pendingSteps,
		ProgressRatio: progressRatio,
		Steps:         steps,
	}, true
}

func buildStartMessage(node topologyNode, suite *suites.Definition, profile string) string {
	switch node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Loading mock assets from mock/ for %s under %s.", node.Name, suite.Title, profile)
	case "script":
		return fmt.Sprintf("[%s] Executing bootstrap scripts declared in scripts/ before exposing dependent services.", node.Name)
	case "load":
		return fmt.Sprintf("[%s] Starting the load harness from load/ with the %s profile and collecting throughput thresholds.", node.Name, profile)
	case "scenario":
		return fmt.Sprintf("[%s] Executing scenario assertions from scenarios/ with the %s profile.", node.Name, profile)
	default:
		return fmt.Sprintf("[%s] Starting container and waiting for health checks from the parsed suite.star topology.", node.Name)
	}
}

func buildHealthyMessage(node topologyNode, suite *suites.Definition, profile string) string {
	switch node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Mock surface is healthy. Exchanges from api/ and mock/ are now routable for %s.", node.Name, suite.Title)
	case "script":
		return fmt.Sprintf("[%s] Script completed successfully. Outputs were registered for the %s execution context.", node.Name, profile)
	case "load":
		return fmt.Sprintf("[%s] Load phase completed. Threshold budgets and synthetic-user ramps stayed within the %s profile.", node.Name, profile)
	case "scenario":
		return fmt.Sprintf("[%s] Scenario passed. Contract assertions and payload policies remained green.", node.Name)
	default:
		return fmt.Sprintf("[%s] Health check passed. Service is ready for downstream containers, load phases, and scenarios.", node.Name)
	}
}

func buildFailureMessage(node topologyNode, suite *suites.Definition) string {
	if node.Kind == "scenario" {
		return fmt.Sprintf("[%s] Assertion failed. Mock exchange drifted from api/ after replay.", node.Name)
	}
	if node.Kind == "load" {
		return fmt.Sprintf("[%s] Load thresholds were exceeded while driving the %s topology under synthetic traffic.", node.Name, suite.Title)
	}
	return fmt.Sprintf("[%s] Container exited with a non-zero status while materializing the %s topology.", node.Name, suite.Title)
}

func buildCommitHash(suiteID, executionID string) string {
	source := strings.ReplaceAll(suiteID+"-"+executionID, "-", "")
	if len(source) >= 10 {
		return source[:10]
	}
	return source + strings.Repeat("a", 10-len(source))
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalSeconds := int(duration.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", totalSeconds/60, totalSeconds%60)
}

func toExecutionProfiles(input []suites.ProfileOption) []ProfileOption {
	output := make([]ProfileOption, len(input))
	for index, profile := range input {
		output[index] = ProfileOption{
			FileName:    profile.FileName,
			Label:       profile.Label,
			Description: profile.Description,
			Default:     profile.Default,
		}
	}
	return output
}

func buildExecutionSuite(suite suites.Definition) ExecutionSuite {
	renderedSurfaces := cloneExecutionSurfaces(suite.APISurfaces)
	for surfaceIndex := range renderedSurfaces {
		for operationIndex := range renderedSurfaces[surfaceIndex].Operations {
			for exchangeIndex := range renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges {
				renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = mocking.PreviewExchange(
					suite,
					renderedSurfaces[surfaceIndex],
					renderedSurfaces[surfaceIndex].Operations[operationIndex],
					renderedSurfaces[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex],
				)
			}
		}
	}

	return ExecutionSuite{
		ID:          suite.ID,
		Title:       suite.Title,
		Repository:  suite.Repository,
		SuiteStar:   suite.SuiteStar,
		Profiles:    toExecutionProfiles(suite.Profiles),
		Folders:     cloneExecutionFolders(suite.Folders),
		SourceFiles: cloneExecutionSourceFiles(suite.SourceFiles),
		APISurfaces: renderedSurfaces,
	}
}

func cloneExecutionSuite(input ExecutionSuite) ExecutionSuite {
	output := input
	output.Profiles = append([]ProfileOption{}, input.Profiles...)
	output.Folders = cloneExecutionFolders(input.Folders)
	output.SourceFiles = cloneExecutionSourceFiles(input.SourceFiles)
	output.APISurfaces = cloneExecutionSurfaces(input.APISurfaces)
	return output
}

func cloneExecutionFolders(input []suites.FolderEntry) []suites.FolderEntry {
	output := make([]suites.FolderEntry, len(input))
	for index, folder := range input {
		output[index] = folder
		output[index].Files = append([]string{}, folder.Files...)
	}
	return output
}

func cloneExecutionSourceFiles(input []suites.SourceFile) []suites.SourceFile {
	output := make([]suites.SourceFile, len(input))
	copy(output, input)
	return output
}

func cloneExecutionSurfaces(input []suites.APISurface) []suites.APISurface {
	output := make([]suites.APISurface, len(input))
	for surfaceIndex, surface := range input {
		output[surfaceIndex] = surface
		output[surfaceIndex].Operations = make([]suites.APIOperation, len(surface.Operations))
		for operationIndex, operation := range surface.Operations {
			output[surfaceIndex].Operations[operationIndex] = operation
			output[surfaceIndex].Operations[operationIndex].MockMetadata = cloneExecutionMockMetadata(operation.MockMetadata)
			output[surfaceIndex].Operations[operationIndex].Exchanges = make([]suites.ExchangeExample, len(operation.Exchanges))
			for exchangeIndex, exchange := range operation.Exchanges {
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex] = exchange
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].When = append([]suites.MatchCondition{}, exchange.When...)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].RequestHeaders = append([]suites.Header{}, exchange.RequestHeaders...)
				output[surfaceIndex].Operations[operationIndex].Exchanges[exchangeIndex].ResponseHeaders = append([]suites.Header{}, exchange.ResponseHeaders...)
			}
		}
	}
	return output
}

func cloneExecutionMockMetadata(input suites.MockOperationMetadata) suites.MockOperationMetadata {
	output := input
	output.ParameterConstraints = append([]suites.ParameterConstraint{}, input.ParameterConstraints...)
	output.Fallback = cloneExecutionFallback(input.Fallback)
	output.State = cloneExecutionState(input.State)
	return output
}

func cloneExecutionFallback(input *suites.MockFallback) *suites.MockFallback {
	if input == nil {
		return nil
	}

	output := *input
	output.Headers = append([]suites.Header{}, input.Headers...)
	return &output
}

func cloneExecutionState(input *suites.MockState) *suites.MockState {
	if input == nil {
		return nil
	}

	output := *input
	if len(input.Defaults) > 0 {
		output.Defaults = make(map[string]string, len(input.Defaults))
		for key, value := range input.Defaults {
			output.Defaults[key] = value
		}
	}
	output.Transitions = make([]suites.MockStateTransition, len(input.Transitions))
	for index, transition := range input.Transitions {
		output.Transitions[index] = transition
		if len(transition.Set) > 0 {
			output.Transitions[index].Set = make(map[string]string, len(transition.Set))
			for key, value := range transition.Set {
				output.Transitions[index].Set[key] = value
			}
		}
		output.Transitions[index].Delete = append([]string{}, transition.Delete...)
		if len(transition.Increment) > 0 {
			output.Transitions[index].Increment = make(map[string]int, len(transition.Increment))
			for key, value := range transition.Increment {
				output.Transitions[index].Increment[key] = value
			}
		}
	}
	return &output
}

func seedExecutionMetadata() map[string]suiteRuntimeMeta {
	return map[string]suiteRuntimeMeta{
		"payment-suite": {
			Author:         "Nora Chen",
			Branch:         "release/payments-v2",
			Message:        "Promote the payment environment with deterministic fraud routing and wiremock-backed checkout paths.",
			FailureTarget:  "checkout-smoke",
			DefaultTrigger: "Manual",
		},
		"fleet-control-room": {
			Author:         "Ari Levin",
			Branch:         "feature/planner-observability",
			Message:        "Run the fleet control topology against mocked telemetry bursts before planner rollout.",
			FailureTarget:  "fleet-smoke",
			DefaultTrigger: "Manual",
		},
		"identity-broker": {
			Author:         "Sam Okafor",
			Branch:         "canary/session-cache",
			Message:        "Validate multi-provider login behavior with strict mock assertions and seeded realms.",
			FailureTarget:  "login-smoke",
			DefaultTrigger: "CI",
		},
		"storefront-browser-lab": {
			Author:         "Lena Hart",
			Branch:         "feature/browser-checkout-lab",
			Message:        "Exercise the storefront browser lane with Playwright, mock APIs, and Kafka-backed checkout events.",
			FailureTarget:  "playwright-checkout",
			DefaultTrigger: "Manual",
		},
		"soap-claims-hub": {
			Author:         "Maya Ortiz",
			Branch:         "feature/soap-bridge-modernization",
			Message:        "Validate SOAP envelopes through the APISIX sidecar before exposing the partner claims bridge.",
			FailureTarget:  "claims-smoke",
			DefaultTrigger: "Manual",
		},
	}
}

func filterOut(items []string, target string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item != target {
			result = append(result, item)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
