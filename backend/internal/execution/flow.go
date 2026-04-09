package execution

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func isKnownStepStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "pending", "running", "healthy", "failed", "skipped":
		return true
	default:
		return false
	}
}

func isTerminalStepStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "healthy", "failed", "skipped":
		return true
	default:
		return false
	}
}

func buildStepStatus(topology []suites.TopologyNode, events []ExecutionEvent) (map[string]string, int) {
	statuses := make(map[string]string, len(topology))
	for _, node := range topology {
		statuses[node.ID] = "pending"
	}

	for _, event := range events {
		source := strings.TrimSpace(event.Source)
		if source == "" {
			continue
		}
		if _, exists := statuses[source]; !exists {
			continue
		}
		if !isKnownStepStatus(event.Status) {
			continue
		}
		statuses[source] = event.Status
	}

	completed := 0
	for _, status := range statuses {
		if isTerminalStepStatus(status) {
			completed++
		}
	}

	return statuses, completed
}

func cloneStepStatuses(statuses map[string]string) map[string]string {
	if len(statuses) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(statuses))
	for key, value := range statuses {
		cloned[key] = value
	}
	return cloned
}

func (s *Service) ensureStepStatusLocked(item *executionState) {
	if item == nil {
		return
	}
	if len(item.stepStatus) > 0 {
		return
	}

	statuses, completed := buildStepStatus(item.record.Suite.Topology, item.record.Events)
	item.stepStatus = statuses
	item.completed = completed
}

func topologyNodeByID(suite *suites.Definition, nodeID string) *topologyNode {
	if suite == nil {
		return nil
	}
	for index := range suite.Topology {
		if suite.Topology[index].ID == nodeID {
			return &suite.Topology[index]
		}
	}
	return nil
}

func (s *Service) executionHasFailed(executionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	return item != nil && item.record.Status == "Failed"
}

func (s *Service) executionTerminalError(executionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return nil
	}
	if item.terminalErr != nil {
		return item.terminalErr
	}
	if item.record.Status == "Failed" {
		return errors.New("execution failed")
	}
	return nil
}

func (s *Service) nodeBelongsToFailurePath(executionID string, suite *suites.Definition, nodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return false
	}
	s.ensureStepStatusLocked(item)
	return s.isFailurePathNodeLocked(item, suite, nodeID, map[string]bool{})
}

func (s *Service) shouldSkipNode(executionID string, suite *suites.Definition, node topologyNode) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return "", false
	}
	s.ensureStepStatusLocked(item)

	if len(node.OnFailure) > 0 {
		triggered := false
		for _, dependencyID := range node.OnFailure {
			if item.stepStatus[dependencyID] == "failed" {
				triggered = true
				break
			}
		}
		if !triggered {
			return fmt.Sprintf("[%s] Skipped because its on_failure conditions were not triggered.", node.Name), true
		}
	}

	for _, dependencyID := range node.DependsOn {
		dependencyStatus := item.stepStatus[dependencyID]
		switch dependencyStatus {
		case "skipped":
			return fmt.Sprintf("[%s] Skipped because dependency %q was skipped.", node.Name, dependencyID), true
		case "failed":
			if containsStepReference(node.OnFailure, dependencyID) {
				continue
			}
			dependency := topologyNodeByID(suite, dependencyID)
			if dependency == nil || !dependency.ContinueOnFailure {
				return fmt.Sprintf("[%s] Skipped because dependency %q failed.", node.Name, dependencyID), true
			}
		}
	}

	if item.record.Status == "Failed" && !s.isFailurePathNodeLocked(item, suite, node.ID, map[string]bool{}) {
		return fmt.Sprintf("[%s] Skipped after a fatal failure in another branch.", node.Name), true
	}

	return "", false
}

func (s *Service) isFailurePathNodeLocked(item *executionState, suite *suites.Definition, nodeID string, visiting map[string]bool) bool {
	if item == nil || suite == nil {
		return false
	}
	if visiting[nodeID] {
		return false
	}
	visiting[nodeID] = true
	defer delete(visiting, nodeID)

	node := topologyNodeByID(suite, nodeID)
	if node == nil {
		return false
	}

	if len(node.OnFailure) > 0 {
		for _, dependencyID := range node.OnFailure {
			if item.stepStatus[dependencyID] == "failed" {
				return true
			}
		}
	}

	for _, dependencyID := range node.DependsOn {
		if s.isFailurePathNodeLocked(item, suite, dependencyID, visiting) {
			return true
		}
	}

	return false
}

func (s *Service) markNodeHealthy(executionID, nodeID, message string) bool {
	return s.markNodeTerminal(executionID, nodeID, "healthy", message, "info", false)
}

func (s *Service) markNodeSkipped(executionID, nodeID, message string) bool {
	return s.markNodeTerminal(executionID, nodeID, "skipped", message, "warn", false)
}

func (s *Service) markNodeFailed(executionID, nodeID, message string, hardFailure bool) bool {
	return s.markNodeTerminal(executionID, nodeID, "failed", message, "error", hardFailure)
}

func (s *Service) markNodeTerminal(executionID, nodeID, status, message, level string, hardFailure bool) bool {
	var (
		streamEvent   StreamEvent
		subscribers   []chan StreamEvent
		cancelRunning bool
		finished      bool
		shouldPersist bool
	)

	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return false
	}
	s.ensureStepStatusLocked(item)

	if isTerminalStepStatus(item.stepStatus[nodeID]) {
		s.mu.Unlock()
		return item.completed >= item.total && item.total > 0
	}

	item.stepStatus[nodeID] = status
	item.completed++
	item.record.UpdatedAt = time.Now().UTC()

	if hardFailure {
		if item.record.Status != "Failed" {
			item.record.Status = "Failed"
			cancelRunning = true
		}
		if strings.TrimSpace(message) != "" && item.terminalErr == nil {
			item.terminalErr = errors.New(message)
		}
	}

	if item.completed >= item.total {
		finished = true
		if item.record.Status != "Failed" {
			item.record.Status = "Healthy"
			item.terminalErr = nil
		}
	}

	event := ExecutionEvent{
		ID:        nodeID + "-" + status,
		Source:    nodeID,
		Timestamp: fmt.Sprintf("%02d:%02d", len(item.record.Events)/60, len(item.record.Events)%60),
		Text:      message,
		Status:    status,
		Level:     level,
	}
	item.record.Events = append(item.record.Events, event)
	streamEvent = StreamEvent{
		ID:              len(item.record.Events),
		ExecutionID:     executionID,
		ExecutionStatus: item.record.Status,
		Duration:        s.durationLocked(item),
		UpdatedAt:       item.record.UpdatedAt,
		Event:           event,
	}
	subscribers = collectSubscribers(s.subs[executionID])
	shouldPersist = true
	s.mu.Unlock()

	if cancelRunning {
		s.queue.CancelRunningGroup(executionID)
	}
	s.publish(streamEvent, subscribers)
	s.appendLog(executionID, event)
	if shouldPersist {
		s.persistExecutionRuntime()
		s.syncObservers(executionID)
	}

	return finished
}

func containsStepReference(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
