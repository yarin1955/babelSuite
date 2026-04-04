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
	for i := range nodes {
		nodes[i].Order = i + 1
	}
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
