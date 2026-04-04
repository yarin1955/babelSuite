package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/queue"
	"github.com/babelsuite/babelsuite/internal/runner"
	"github.com/babelsuite/babelsuite/internal/suites"
)

func NewService(source suiteSource, observers ...Observer) *Service {
	return NewServiceWithPlatform(source, nil, observers...)
}

func (s *Service) Close() {
	if s.signals != nil {
		s.signals.shutdown()
	}
	s.cancel()
	s.queue.Close()
}

func (s *Service) ListLaunchSuites() []LaunchSuite {
	backends := s.backendOptions()
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
			Backends:    append([]BackendOption{}, backends...),
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

	selectedBackend, err := s.resolveBackend(ctx, request.Backend)
	if err != nil {
		s.noteRejectedLaunch(ctx, suite.ID, "backend_unavailable")
		return nil, err
	}

	executionID := "run-" + uuid.NewString()[:8]
	startedAt := time.Now().UTC()
	state := &executionState{
		record: ExecutionRecord{
			ID:        executionID,
			Suite:     buildExecutionSuite(*suite),
			Profile:   profile,
			BackendID: selectedBackend.option.ID,
			Backend:   selectedBackend.option.Label,
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

	topology, err := parseSuiteTopology(suite.SuiteStar)
	if err != nil {
		s.noteRejectedLaunch(ctx, suite.ID, "invalid_topology")
		return nil, err
	}
	state.total = len(topology)

	s.mu.Lock()
	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.mu.Unlock()
	s.logs.Open(executionID)
	s.persistExecutionRuntime()
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
				return s.runNode(ctx, executionID, suite, profile, selectedBackend.backend, node)
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

func (s *Service) runNode(ctx context.Context, executionID string, suite *suites.Definition, profile string, backend runner.Backend, node topologyNode) error {
	stepCtx, stepSpan, stepStartedAt := s.beginStepObservation(s.stepContext(executionID), executionID, suite, profile, node)

	s.appendEvent(executionID, ExecutionEvent{
		ID:        node.ID + "-start",
		Source:    node.ID,
		Timestamp: s.nextTimestamp(executionID),
		Text:      buildStartMessage(node, suite, profile),
		Status:    "running",
		Level:     "info",
	})

	err := backend.Run(stepCtx, runner.StepSpec{
		ExecutionID:     executionID,
		SuiteID:         suite.ID,
		SuiteTitle:      suite.Title,
		SuiteRepository: suite.Repository,
		Profile:         profile,
		Trigger:         s.executionTrigger(executionID),
		BackendID:       s.executionBackendID(executionID),
		BackendLabel:    s.executionBackendLabel(executionID),
		BackendKind:     backend.Kind(),
		StepIndex:       node.Order,
		TotalSteps:      len(parseSuiteTopologyOrEmpty(suite.SuiteStar)),
		LeaseTTL:        8 * time.Second,
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
	s.persistExecutionRuntime()
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
		go s.persistExecutionRuntime()
		return true
	}
	go s.persistExecutionRuntime()
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
	s.persistExecutionRuntime()
	s.syncObservers(executionID)
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
	s.persistExecutionRuntime()
}

func (s *Service) summaryLocked(item *executionState) ExecutionSummary {
	return ExecutionSummary{
		ID:         item.record.ID,
		SuiteID:    item.record.Suite.ID,
		SuiteTitle: item.record.Suite.Title,
		Profile:    item.record.Profile,
		BackendID:  item.record.BackendID,
		Backend:    item.record.Backend,
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

func (s *Service) executionTrigger(executionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item := s.executions[executionID]; item != nil {
		return item.record.Trigger
	}
	return ""
}

func (s *Service) executionBackendID(executionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item := s.executions[executionID]; item != nil {
		return item.record.BackendID
	}
	return ""
}

func (s *Service) executionBackendLabel(executionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item := s.executions[executionID]; item != nil {
		return item.record.Backend
	}
	return ""
}
