package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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

func (e topologyResolutionError) Unwrap() error {
	return e.cause
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
		nextReady := make([]string, 0)

		for _, id := range ready {
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
	result := make([]string, 0, len(dependsOn))
	for _, dep := range dependsOn {
		duplicate := false
		for _, existing := range result {
			if existing == dep {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, dep)
		}
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
			Timestamp: formatDuration(time.Duration(len(events)) * 2 * time.Second),
			Text:      buildStartMessage(node, suite, profile),
			Status:    stepStatusRunning,
			Level:     "info",
		})

		if status == "Failed" && node.ID == meta.FailureTarget {
			events = append(events, ExecutionEvent{
				ID:        node.ID + "-failed",
				Source:    node.ID,
				Timestamp: formatDuration(time.Duration(len(events)) * 2 * time.Second),
				Text:      buildFailureMessage(node, suite),
				Status:    stepStatusFailed,
				Level:     "error",
			})
			break
		}

		events = append(events, ExecutionEvent{
			ID:        node.ID + "-healthy",
			Source:    node.ID,
			Timestamp: formatDuration(time.Duration(len(events)) * 2 * time.Second),
			Text:      buildHealthyMessage(node, suite, profile),
			Status:    stepStatusHealthy,
			Level:     "info",
		})
	}
	return events
}

func (s *Service) snapshotExecution(executionID string) (Snapshot, bool) {
	s.mu.Lock()
	item := s.executions[executionID]
	if item == nil {
		s.mu.Unlock()
		return Snapshot{}, false
	}
	s.ensureStepStatusLocked(item)
	topology := cloneExecutionTopology(item.record.Suite.Topology)
	statuses := cloneStepStatuses(item.stepStatus)
	record := item.record
	duration := s.durationLocked(item)
	s.mu.Unlock()

	if len(statuses) == 0 {
		statuses, _ = buildStepStatus(record.Suite.Topology, record.Events)
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
		case stepStatusRunning:
			runningSteps++
		case stepStatusHealthy:
			healthySteps++
		case stepStatusFailed:
			failedSteps++
		case stepStatusSkipped:
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
		ID:            record.ID,
		SuiteID:       record.Suite.ID,
		SuiteTitle:    record.Suite.Title,
		Profile:       record.Profile,
		BackendID:     record.BackendID,
		Backend:       record.Backend,
		Trigger:       record.Trigger,
		Status:        record.Status,
		Duration:      duration,
		StartedAt:     record.StartedAt,
		UpdatedAt:     record.UpdatedAt,
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
	case suites.NodeKindMock:
		return fmt.Sprintf("[%s] Loading mock assets from api/ and mock/ for %s under %s.", node.Name, suite.Title, profile)
	case suites.NodeKindService:
		if isCompatibilityVariant(node.Variant) {
			return fmt.Sprintf("[%s] Starting compatibility service assets from services/ under the %s profile.", node.Name, profile)
		}
		return fmt.Sprintf("[%s] Starting background service infrastructure declared in suite.star under the %s profile.", node.Name, profile)
	case suites.NodeKindTask:
		return fmt.Sprintf("[%s] Executing one-shot task assets from tasks/ before exposing dependent services.", node.Name)
	case suites.NodeKindTraffic:
		return fmt.Sprintf("[%s] Starting the traffic harness from traffic/ with the %s profile and collecting throughput thresholds.", node.Name, profile)
	case suites.NodeKindTest:
		return fmt.Sprintf("[%s] Executing verification assets from tests/ with the %s profile.", node.Name, profile)
	default:
		return fmt.Sprintf("[%s] Starting workload and waiting for health checks from the parsed suite.star topology.", node.Name)
	}
}

func buildHealthyMessage(node topologyNode, suite *suites.Definition, profile string) string {
	switch node.Kind {
	case suites.NodeKindMock:
		return fmt.Sprintf("[%s] Mock surface is healthy. Exchanges from api/ and mock/ are now routable for %s.", node.Name, suite.Title)
	case suites.NodeKindService:
		if isCompatibilityVariant(node.Variant) {
			return fmt.Sprintf("[%s] Compatibility service is healthy and reachable for downstream checks in the %s profile.", node.Name, profile)
		}
		return fmt.Sprintf("[%s] Background service is healthy and reachable for downstream steps in the %s profile.", node.Name, profile)
	case suites.NodeKindTask:
		return fmt.Sprintf("[%s] Task completed successfully. Outputs were registered for the %s execution context.", node.Name, profile)
	case suites.NodeKindTraffic:
		return fmt.Sprintf("[%s] Traffic phase completed. Threshold budgets and synthetic-user ramps stayed within the %s profile.", node.Name, profile)
	case suites.NodeKindTest:
		return fmt.Sprintf("[%s] Test phase passed. Contract assertions and payload policies remained green.", node.Name)
	default:
		return fmt.Sprintf("[%s] Health check passed. Service is ready for downstream services, traffic phases, and tests.", node.Name)
	}
}

func buildFailureMessage(node topologyNode, suite *suites.Definition) string {
	switch node.Kind {
	case suites.NodeKindTest:
		return fmt.Sprintf("[%s] Assertion failed. Mock exchange drifted from api/ after replay.", node.Name)
	case suites.NodeKindTraffic:
		return fmt.Sprintf("[%s] Traffic thresholds were exceeded while driving the %s topology under synthetic traffic.", node.Name, suite.Title)
	case suites.NodeKindService:
		if isCompatibilityVariant(node.Variant) {
			return fmt.Sprintf("[%s] Compatibility service exited unexpectedly while supporting the %s topology.", node.Name, suite.Title)
		}
		return fmt.Sprintf("[%s] Background service exited unexpectedly while supporting the %s topology.", node.Name, suite.Title)
	case suites.NodeKindTask:
		return fmt.Sprintf("[%s] Task exited with a non-zero status while preparing the %s topology.", node.Name, suite.Title)
	default:
		return fmt.Sprintf("[%s] Workload exited with a non-zero status while materializing the %s topology.", node.Name, suite.Title)
	}
}

func isCompatibilityVariant(variant string) bool {
	return variant == suites.VariantServicePrism ||
		variant == suites.VariantServiceWiremock ||
		variant == suites.VariantServiceCustom
}

func buildCommitHash(suiteID, executionID string) string {
	sum := sha256.Sum256([]byte(suiteID + "-" + executionID))
	return hex.EncodeToString(sum[:])[:10]
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalSeconds := int(duration.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", totalSeconds/60, totalSeconds%60)
}
