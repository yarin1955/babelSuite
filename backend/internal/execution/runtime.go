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

func (s *Service) ConfigureMockResetter(resetter mockResetter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mockResetter = resetter
}

func (s *Service) Close() {
	if s.signals != nil {
		s.signals.shutdown()
	}
	s.cancel()
	s.queue.Close()
}

func (s *Service) ResolveRef(ref string) (*LaunchSuite, error) {
	suite, err := s.suiteSource.Resolve(ref)
	if err != nil {
		return nil, ErrSuiteNotFound
	}
	backends := s.backendOptions()
	result := LaunchSuite{
		ID:          suite.ID,
		Title:       suite.Title,
		Repository:  suite.Repository,
		Description: suite.Description,
		Provider:    suite.Provider,
		Status:      suite.Status,
		Profiles:    toExecutionProfiles(suite.Profiles),
		Backends:    append([]BackendOption{}, backends...),
	}
	return &result, nil
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
	record.Artifacts = cloneExecutionArtifacts(item.record.Artifacts)
	record.Suite = cloneExecutionSuite(item.record.Suite)
	return &record, nil
}

func (s *Service) CreateExecution(ctx context.Context, request CreateRequest) (*ExecutionSummary, error) {
	suiteID := strings.TrimSpace(request.SuiteID)
	s.noteLaunch(ctx, suiteID)

	suite, err := s.suiteSource.Get(suiteID)
	if err != nil {
		suite, err = s.suiteSource.Resolve(suiteID)
	}
	if err != nil {
		s.noteRejectedLaunch(ctx, suiteID, "suite_not_found")
		return nil, ErrSuiteNotFound
	}
	resolved, err := suites.ResolveRuntime(*suite, s.suiteSource.List())
	if err != nil {
		s.noteRejectedLaunch(ctx, suite.ID, "invalid_topology")
		return nil, ErrInvalidTopology
	}
	suite.Topology = resolved.Nodes
	suite.ResolvedDependencies = resolved.Dependencies
	suite.TopologyError = ""
	meta := s.suiteMeta[suite.ID]

	profile := strings.TrimSpace(request.Profile)
	if profile == "" {
		profile = defaultProfile(suite.Profiles)
	}
	if !suiteHasProfile(suite.Profiles, profile) {
		s.noteRejectedLaunch(ctx, suite.ID, "profile_not_found")
		return nil, ErrProfileNotFound
	}

	runtimeOverlay, err := s.resolveExecutionRuntimeOverlay(ctx, suite.ID, profile)
	if err != nil {
		s.noteRejectedLaunch(ctx, suite.ID, "profile_runtime_error")
		return nil, err
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
			Artifacts: []ExecutionArtifact{},
		},
		runtime:    runtimeOverlay,
		stepStatus: make(map[string]string, len(resolved.Nodes)),
	}
	state.total = len(resolved.Nodes)
	for _, node := range resolved.Nodes {
		state.stepStatus[node.ID] = "pending"
	}

	s.mu.Lock()
	s.executions[executionID] = state
	s.order = append(s.order, executionID)
	s.mu.Unlock()
	s.logs.Open(executionID)
	s.persistExecutionRuntime()
	s.beginRunObservation(ctx, state)

	tasks := make([]queue.Task, 0, len(resolved.Nodes))
	taskIDs := make(map[string]string, len(resolved.Nodes))
	for _, node := range resolved.Nodes {
		taskIDs[node.ID] = executionID + ":" + node.ID
	}
	for _, node := range resolved.Nodes {
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

	if reason, skip := s.shouldSkipNode(executionID, suite, node); skip {
		finished := s.markNodeSkipped(executionID, node.ID, reason)
		s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, nil)
		if finished {
			s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
		}
		return nil
	}

	s.appendEvent(executionID, ExecutionEvent{
		ID:        node.ID + "-start",
		Source:    node.ID,
		Timestamp: s.nextTimestamp(executionID),
		Text:      buildStartMessage(node, suite, profile),
		Status:    "running",
		Level:     "info",
	})

	if err := s.resetMockState(stepCtx, executionID, suite, node); err != nil {
		message := fmt.Sprintf("[%s] Mock reset failed: %v", node.Name, err)
		finished := s.markNodeFailed(executionID, node.ID, message, !node.ContinueOnFailure)
		s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, err)
		if finished {
			s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
		}
		if node.ContinueOnFailure {
			s.appendEvent(executionID, ExecutionEvent{
				ID:        node.ID + "-continued",
				Source:    node.ID,
				Timestamp: s.nextTimestamp(executionID),
				Text:      fmt.Sprintf("[%s] continue_on_failure is enabled; downstream nodes may continue.", node.Name),
				Status:    "failed",
				Level:     "warn",
			})
			return nil
		}
		return err
	}

	collectedFiles := make(map[string][]byte)

	err := backend.Run(stepCtx, runner.StepSpec{
		ExecutionID:      executionID,
		SuiteID:          suite.ID,
		SuiteTitle:       suite.Title,
		SuiteRepository:  suite.Repository,
		Profile:          profile,
		RuntimeProfile:   firstNonEmpty(node.RuntimeProfile, profile),
		Env:              s.resolveNodeRuntimeEnv(executionID, node),
		Headers:          cloneRuntimeMap(node.RuntimeHeaders),
		Trigger:          s.executionTrigger(executionID),
		BackendID:        s.executionBackendID(executionID),
		BackendLabel:     s.executionBackendLabel(executionID),
		BackendKind:      backend.Kind(),
		SourceSuiteID:    firstNonEmpty(node.SourceSuiteID, suite.ID),
		SourceSuiteTitle: firstNonEmpty(node.SourceSuiteTitle, suite.Title),
		SourceRepository: firstNonEmpty(node.SourceRepository, suite.Repository),
		SourceVersion:    firstNonEmpty(node.SourceVersion, suite.Version),
		ResolvedRef:      node.ResolvedRef,
		Digest:           node.Digest,
		DependencyAlias:  node.DependencyAlias,
		StepIndex:        node.Order,
		TotalSteps:       len(suite.Topology),
		LeaseTTL:         8 * time.Second,
		Load:             suitesCloneLoadSpec(node.Load),
		Evaluation:       cloneNodeEvaluation(node.Evaluation),
		OnFailure:        append([]string{}, node.OnFailure...),
		ArtifactExports:  cloneNodeArtifactExports(node.ArtifactExports),
		OnArtifact: func(path string, content []byte) {
			if len(content) > 0 {
				collectedFiles[path] = content
			}
		},
		GatewayURL: resolveGatewayURL(executionID, suite),
		Node: runner.StepNode{
			ID:        node.ID,
			Name:      node.Name,
			Kind:      node.Kind,
			Variant:   node.Variant,
			DependsOn: append([]string{}, node.DependsOn...),
		},
	}, func(line logstream.Line) {
		s.appendRunnerLog(executionID, node.ID, line)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) && s.executionHasFailed(executionID) && !s.nodeBelongsToFailurePath(executionID, suite, node.ID) {
			finished := s.markNodeSkipped(executionID, node.ID, fmt.Sprintf("[%s] Canceled after a fatal failure in another node.", node.Name))
			s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, nil)
			if finished {
				s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
			}
			return nil
		}
		if errors.Is(err, context.Canceled) {
			finished := s.markNodeFailed(executionID, node.ID, fmt.Sprintf("[%s] Execution canceled before the node became healthy.", node.Name), true)
			s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, context.Canceled)
			if finished {
				s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
			}
			return context.Canceled
		}
		message := fmt.Sprintf("[%s] Runner failed: %v", node.Name, err)
		finished := s.markNodeFailed(executionID, node.ID, message, !node.ContinueOnFailure)
		s.registerStepArtifacts(executionID, node, "failed", collectedFiles)
		s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, err)
		if finished {
			s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
		}
		if node.ContinueOnFailure {
			s.appendEvent(executionID, ExecutionEvent{
				ID:        node.ID + "-continued",
				Source:    node.ID,
				Timestamp: s.nextTimestamp(executionID),
				Text:      fmt.Sprintf("[%s] continue_on_failure is enabled; downstream nodes may continue.", node.Name),
				Status:    "failed",
				Level:     "warn",
			})
			return nil
		}
		return err
	}

	finished := s.markNodeHealthy(executionID, node.ID, buildHealthyMessage(node, suite, profile))
	s.registerStepArtifacts(executionID, node, "healthy", collectedFiles)
	s.finishStepObservation(stepCtx, stepSpan, stepStartedAt, executionID, suite, profile, node, nil)
	if finished {
		s.finishExecutionObservation(executionID, s.executionTerminalError(executionID))
	}

	return nil
}

func cloneNodeArtifactExports(input []suites.ArtifactExport) []runner.ArtifactExport {
	if len(input) == 0 {
		return nil
	}

	output := make([]runner.ArtifactExport, len(input))
	for index, item := range input {
		output[index] = runner.ArtifactExport{
			Path:   item.Path,
			Name:   item.Name,
			On:     item.On,
			Format: item.Format,
		}
	}
	return output
}

func cloneNodeEvaluation(input *suites.StepEvaluation) *suites.StepEvaluation {
	if input == nil {
		return nil
	}

	output := *input
	output.ExpectLogs = append([]string{}, input.ExpectLogs...)
	output.FailOnLogs = append([]string{}, input.FailOnLogs...)
	if input.ExpectExit != nil {
		value := *input.ExpectExit
		output.ExpectExit = &value
	}
	return &output
}

func cloneRuntimeMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (s *Service) appendEvent(executionID string, event ExecutionEvent) {
	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return
	}
	s.ensureStepStatusLocked(item)
	if source := strings.TrimSpace(event.Source); source != "" && isKnownStepStatus(event.Status) {
		if _, exists := item.stepStatus[source]; exists {
			item.stepStatus[source] = event.Status
		}
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

// resolveGatewayURL returns the APISIX sidecar address for this execution by
// locating the suite's mock node and deriving its container hostname.
// The container name follows the same pattern used by the Docker runner:
// babel-{executionID}-{nodeID}, listening on APISIX's default port 9080.
// Returns an empty string when the suite has no mock node.
func resolveGatewayURL(executionID string, suite *suites.Definition) string {
	if suite == nil {
		return ""
	}
	for _, node := range suite.Topology {
		if strings.TrimSpace(node.Kind) != "mock" {
			continue
		}
		host := "babel-" + sanitizeContainerID(executionID) + "-" + sanitizeContainerID(node.ID)
		return "http://" + host + ":9080"
	}
	return ""
}

// sanitizeContainerID mirrors the runner's sanitizeID so container names match.
func sanitizeContainerID(id string) string {
	var b strings.Builder
	for _, ch := range id {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-':
			b.WriteRune(ch)
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch + 32)
		default:
			b.WriteRune('-')
		}
	}
	s := b.String()
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
}
