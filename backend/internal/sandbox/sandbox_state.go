package sandbox

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func ensureSandbox(groups map[string]*Sandbox, key string) *Sandbox {
	existing, ok := groups[key]
	if ok {
		return existing
	}

	sandbox := &Sandbox{
		SandboxID:         key,
		OrchestratorState: "unknown",
		Containers:        []Container{},
		Networks:          []Network{},
		Volumes:           []Volume{},
		Warnings:          []string{},
	}
	groups[key] = sandbox
	return sandbox
}

func hydrateSandbox(sandbox *Sandbox, labels map[string]string) {
	if labels == nil {
		return
	}

	if sandbox.RunID == "" {
		sandbox.RunID = firstNonEmpty(labels["babelsuite.run_id"], labels["babelsuite.sandbox_id"], labels["babelsuite.execution_id"])
	}
	if sandbox.Suite == "" {
		sandbox.Suite = firstNonEmpty(labels["babelsuite.suite"], labels["babelsuite.suite_name"], labels["babelsuite.suite_id"])
	}
	if sandbox.Owner == "" {
		sandbox.Owner = firstNonEmpty(labels["babelsuite.owner"], labels["babelsuite.user"], labels["babelsuite.initiator"])
	}
	if sandbox.Profile == "" {
		sandbox.Profile = labels["babelsuite.profile"]
	}
	if sandbox.OrchestratorPID == 0 {
		sandbox.OrchestratorPID = parseInt(labels["babelsuite.orchestrator_pid"])
	}
	if sandbox.StartedAt == nil {
		sandbox.StartedAt = parseDockerTime(firstNonEmpty(labels["babelsuite.started_at"], labels["babelsuite.run_started_at"]))
	}
	if sandbox.LastHeartbeatAt == nil {
		sandbox.LastHeartbeatAt = parseDockerTime(firstNonEmpty(labels["babelsuite.last_heartbeat"], labels["babelsuite.heartbeat_at"]))
	}
	if labelIsTrue(labels["babelsuite.zombie"]) {
		sandbox.IsZombie = true
	}
}

func finalizeSandbox(ctx context.Context, checker processChecker, now time.Time, sandbox *Sandbox) {
	sort.Slice(sandbox.Containers, func(i, j int) bool { return sandbox.Containers[i].Name < sandbox.Containers[j].Name })
	sort.Slice(sandbox.Networks, func(i, j int) bool { return sandbox.Networks[i].Name < sandbox.Networks[j].Name })
	sort.Slice(sandbox.Volumes, func(i, j int) bool { return sandbox.Volumes[i].Name < sandbox.Volumes[j].Name })

	for index := range sandbox.Containers {
		if sandbox.StartedAt == nil || (sandbox.Containers[index].StartedAt != nil && sandbox.Containers[index].StartedAt.Before(*sandbox.StartedAt)) {
			sandbox.StartedAt = sandbox.Containers[index].StartedAt
		}
	}

	running := 0
	failed := 0
	for _, item := range sandbox.Containers {
		if item.State == "running" {
			running++
		}
		if item.State == "exited" || item.State == "dead" || item.State == "restarting" {
			failed++
		}
	}

	switch {
	case sandbox.OrchestratorPID > 0 && checker.Alive(ctx, sandbox.OrchestratorPID):
		sandbox.OrchestratorState = "alive"
	case sandbox.OrchestratorPID > 0:
		sandbox.OrchestratorState = "dead"
	case sandbox.LastHeartbeatAt != nil && now.Sub(*sandbox.LastHeartbeatAt) > 2*time.Minute:
		sandbox.OrchestratorState = "stale"
	default:
		sandbox.OrchestratorState = "unknown"
	}

	if !sandbox.IsZombie && running > 0 && (sandbox.OrchestratorState == "dead" || sandbox.OrchestratorState == "stale") {
		sandbox.IsZombie = true
	}

	switch {
	case sandbox.IsZombie:
		sandbox.Status = "Zombie"
	case failed > 0:
		sandbox.Status = "Degraded"
	case running > 0:
		sandbox.Status = "Running"
	case len(sandbox.Containers)+len(sandbox.Networks)+len(sandbox.Volumes) > 0:
		sandbox.Status = "Residual"
	default:
		sandbox.Status = "Unknown"
	}

	if sandbox.Suite == "" {
		sandbox.Suite = "Unattributed sandbox"
	}
	if sandbox.Owner == "" {
		sandbox.Owner = "Unknown owner"
	}
	if sandbox.Profile == "" {
		sandbox.Profile = "No profile label"
	}
	if sandbox.RunID == "" {
		sandbox.Warnings = append(sandbox.Warnings, "Resources are missing babelsuite.run_id, so cleanup is grouped by fallback labels.")
	}
	if sandbox.IsZombie {
		sandbox.Warnings = append(sandbox.Warnings, "The orchestrator appears to be gone while managed containers are still alive.")
	}

	sandbox.CanReap = len(sandbox.Containers)+len(sandbox.Networks)+len(sandbox.Volumes) > 0
	sandbox.Warnings = compactStrings(sandbox.Warnings)
	sandbox.ResourceUsage.CPUPercent = roundFloat(sandbox.ResourceUsage.CPUPercent)
	sandbox.ResourceUsage.MemoryPercent = roundFloat(sandbox.ResourceUsage.MemoryPercent)
	sandbox.Summary = fmt.Sprintf("%d containers, %d networks, %d volumes", len(sandbox.Containers), len(sandbox.Networks), len(sandbox.Volumes))
}

func lookupUsage(stats map[string]ResourceUsage, id, name string) ResourceUsage {
	if usage, ok := stats[id]; ok {
		return usage
	}
	if usage, ok := stats[name]; ok {
		return usage
	}
	for key, usage := range stats {
		if strings.HasPrefix(id, key) || strings.HasPrefix(key, id) {
			return usage
		}
	}
	return ResourceUsage{}
}

func addUsage(total *ResourceUsage, next ResourceUsage) {
	total.CPUPercent += next.CPUPercent
	total.MemoryBytes += next.MemoryBytes
	total.MemoryLimitBytes += next.MemoryLimitBytes
	if total.MemoryLimitBytes > 0 {
		total.MemoryPercent = roundFloat(float64(total.MemoryBytes) / float64(total.MemoryLimitBytes) * 100)
	}
}

func sandboxKey(labels map[string]string, kind, fallback string) string {
	runID := firstNonEmpty(labels["babelsuite.run_id"], labels["babelsuite.sandbox_id"], labels["babelsuite.execution_id"])
	if runID != "" {
		return runID
	}
	return "orphan-" + kind + "-" + slugify(fallback)
}

func sandboxStatusRank(status string) int {
	switch status {
	case "Zombie":
		return 0
	case "Degraded":
		return 1
	case "Running":
		return 2
	case "Residual":
		return 3
	default:
		return 4
	}
}
