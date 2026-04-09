package execution

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

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

func parseSuiteTopology(suiteStar string) ([]topologyNode, error) {
	nodes, err := suites.ResolveTopology(suites.Definition{
		ID:        "inline-suite",
		Title:     "Inline Suite",
		SuiteStar: suiteStar,
	}, nil)
	if err != nil {
		return nil, topologyResolutionError{cause: err}
	}
	return nodes, nil
}

func parseSuiteTopologyOrEmpty(suiteStar string) []topologyNode {
	nodes, err := parseSuiteTopology(suiteStar)
	if err != nil {
		return nil
	}
	return nodes
}

type topologyResolutionError struct {
	cause error
}

func (e topologyResolutionError) Error() string {
	return e.cause.Error()
}

func (e topologyResolutionError) Is(target error) bool {
	return target == ErrInvalidTopology
}

func resolveTopology(nodes []topologyNode) ([]topologyNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	byID := make(map[string]*topologyNode, len(nodes))
	for i := range nodes {
		nodes[i].DependsOn = normalizeTopologyDependencies(nodes[i].DependsOn)
		if _, exists := byID[nodes[i].ID]; exists {
			return nil, &topologyDuplicateNodeError{Node: nodes[i].ID}
		}
		byID[nodes[i].ID] = &nodes[i]
	}

	indegree := make(map[string]int, len(nodes))
	dependants := make(map[string][]string, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		indegree[node.ID] = len(node.DependsOn)
		for _, dependency := range node.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return nil, &topologyMissingDependencyError{Node: node.ID, Dependency: dependency}
			}
			dependants[dependency] = append(dependants[dependency], node.ID)
		}
	}

	ready := make([]string, 0)
	for _, node := range nodes {
		if indegree[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	sortTopologyIDs(ready, byID)

	ordered := make([]topologyNode, 0, len(nodes))
	level := 0
	for len(ready) > 0 {
		current := append([]string{}, ready...)
		nextReady := make([]string, 0)

		for _, id := range current {
			node := *byID[id]
			node.Level = level
			ordered = append(ordered, node)

			for _, dependant := range dependants[id] {
				indegree[dependant]--
				if indegree[dependant] == 0 {
					nextReady = append(nextReady, dependant)
				}
			}
		}

		sortTopologyIDs(nextReady, byID)
		ready = nextReady
		level++
	}

	if len(ordered) != len(nodes) {
		return nil, &topologyCycleError{Path: findTopologyCycle(nodes, byID)}
	}

	return ordered, nil
}

func normalizeTopologyDependencies(dependsOn []string) []string {
	if len(dependsOn) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(dependsOn))
	result := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		if _, exists := seen[dependency]; exists {
			continue
		}
		seen[dependency] = struct{}{}
		result = append(result, dependency)
	}
	return result
}

func sortTopologyIDs(ids []string, byID map[string]*topologyNode) {
	sort.Slice(ids, func(i, j int) bool {
		left := byID[ids[i]]
		right := byID[ids[j]]
		if left == nil || right == nil {
			return ids[i] < ids[j]
		}
		if left.Order != right.Order {
			return left.Order < right.Order
		}
		return left.Name < right.Name
	})
}

func findTopologyCycle(nodes []topologyNode, byID map[string]*topologyNode) []string {
	visiting := make(map[string]bool, len(nodes))
	visited := make(map[string]bool, len(nodes))
	path := make([]string, 0, len(nodes))

	var visit func(string) []string
	visit = func(id string) []string {
		visited[id] = true
		visiting[id] = true
		path = append(path, id)

		node := byID[id]
		if node != nil {
			for _, dependency := range node.DependsOn {
				if !visited[dependency] {
					if cycle := visit(dependency); len(cycle) > 0 {
						return cycle
					}
					continue
				}
				if !visiting[dependency] {
					continue
				}
				start := 0
				for i, stepID := range path {
					if stepID == dependency {
						start = i
						break
					}
				}
				cycle := append([]string{}, path[start:]...)
				cycle = append(cycle, dependency)
				return cycle
			}
		}

		visiting[id] = false
		path = path[:len(path)-1]
		return nil
	}

	for _, node := range nodes {
		if visited[node.ID] {
			continue
		}
		if cycle := visit(node.ID); len(cycle) > 0 {
			return cycle
		}
	}

	return nil
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

func (s *Service) snapshotExecution(executionID string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return Snapshot{}, false
	}
	s.ensureStepStatusLocked(item)

	topology := cloneExecutionTopology(item.record.Suite.Topology)
	statuses := cloneStepStatuses(item.stepStatus)
	if len(statuses) == 0 {
		statuses, _ = buildStepStatus(item.record.Suite.Topology, item.record.Events)
	}

	steps := make([]StepSnapshot, 0, len(topology))
	runningSteps := 0
	healthySteps := 0
	failedSteps := 0
	skippedSteps := 0
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
		case "skipped":
			skippedSteps++
		default:
			pendingSteps++
		}

		steps = append(steps, StepSnapshot{
			ID:        node.ID,
			Name:      node.Name,
			Kind:      node.Kind,
			Status:    status,
			DependsOn: append([]string{}, node.DependsOn...),
			Level:     node.Level,
		})
	}

	progressRatio := 0.0
	if len(steps) > 0 {
		progressRatio = float64(healthySteps+runningSteps+failedSteps+skippedSteps) / float64(len(steps))
	}

	return Snapshot{
		ID:            item.record.ID,
		SuiteID:       item.record.Suite.ID,
		SuiteTitle:    item.record.Suite.Title,
		Profile:       item.record.Profile,
		BackendID:     item.record.BackendID,
		Backend:       item.record.Backend,
		Trigger:       item.record.Trigger,
		Status:        item.record.Status,
		Duration:      s.durationLocked(item),
		StartedAt:     item.record.StartedAt,
		UpdatedAt:     item.record.UpdatedAt,
		TotalSteps:    len(steps),
		RunningSteps:  runningSteps,
		HealthySteps:  healthySteps,
		FailedSteps:   failedSteps,
		SkippedSteps:  skippedSteps,
		PendingSteps:  pendingSteps,
		ProgressRatio: progressRatio,
		Steps:         steps,
	}, true
}

func buildStartMessage(node topologyNode, suite *suites.Definition, profile string) string {
	switch node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Loading mock assets from api/ and mock/ for %s under %s.", node.Name, suite.Title, profile)
	case "service":
		if node.Variant == "service.prism" || node.Variant == "service.wiremock" || node.Variant == "service.custom" {
			return fmt.Sprintf("[%s] Starting compatibility service assets from services/ under the %s profile.", node.Name, profile)
		}
		return fmt.Sprintf("[%s] Starting background service infrastructure declared in suite.star under the %s profile.", node.Name, profile)
	case "task":
		return fmt.Sprintf("[%s] Executing one-shot task assets from tasks/ before exposing dependent services.", node.Name)
	case "traffic":
		return fmt.Sprintf("[%s] Starting the traffic harness from traffic/ with the %s profile and collecting throughput thresholds.", node.Name, profile)
	case "test":
		return fmt.Sprintf("[%s] Executing verification assets from tests/ with the %s profile.", node.Name, profile)
	default:
		return fmt.Sprintf("[%s] Starting workload and waiting for health checks from the parsed suite.star topology.", node.Name)
	}
}

func buildHealthyMessage(node topologyNode, suite *suites.Definition, profile string) string {
	switch node.Kind {
	case "mock":
		return fmt.Sprintf("[%s] Mock surface is healthy. Exchanges from api/ and mock/ are now routable for %s.", node.Name, suite.Title)
	case "service":
		if node.Variant == "service.prism" || node.Variant == "service.wiremock" || node.Variant == "service.custom" {
			return fmt.Sprintf("[%s] Compatibility service is healthy and reachable for downstream checks in the %s profile.", node.Name, profile)
		}
		return fmt.Sprintf("[%s] Background service is healthy and reachable for downstream steps in the %s profile.", node.Name, profile)
	case "task":
		return fmt.Sprintf("[%s] Task completed successfully. Outputs were registered for the %s execution context.", node.Name, profile)
	case "traffic":
		return fmt.Sprintf("[%s] Traffic phase completed. Threshold budgets and synthetic-user ramps stayed within the %s profile.", node.Name, profile)
	case "test":
		return fmt.Sprintf("[%s] Test phase passed. Contract assertions and payload policies remained green.", node.Name)
	default:
		return fmt.Sprintf("[%s] Health check passed. Service is ready for downstream services, traffic phases, and tests.", node.Name)
	}
}

func buildFailureMessage(node topologyNode, suite *suites.Definition) string {
	if node.Kind == "test" {
		return fmt.Sprintf("[%s] Assertion failed. Mock exchange drifted from api/ after replay.", node.Name)
	}
	if node.Kind == "traffic" {
		return fmt.Sprintf("[%s] Traffic thresholds were exceeded while driving the %s topology under synthetic traffic.", node.Name, suite.Title)
	}
	if node.Kind == "service" {
		if node.Variant == "service.prism" || node.Variant == "service.wiremock" || node.Variant == "service.custom" {
			return fmt.Sprintf("[%s] Compatibility service exited unexpectedly while supporting the %s topology.", node.Name, suite.Title)
		}
		return fmt.Sprintf("[%s] Background service exited unexpectedly while supporting the %s topology.", node.Name, suite.Title)
	}
	if node.Kind == "task" {
		return fmt.Sprintf("[%s] Task exited with a non-zero status while preparing the %s topology.", node.Name, suite.Title)
	}
	return fmt.Sprintf("[%s] Workload exited with a non-zero status while materializing the %s topology.", node.Name, suite.Title)
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
