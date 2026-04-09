package environments

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

func (s *Service) Snapshot(ctx context.Context) (*Inventory, error) {
	containers, networks, volumes, dockerAvailable, warnings, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}

	if !dockerAvailable {
		return &Inventory{
			DockerAvailable: false,
			UpdatedAt:       s.now(),
			Summary:         InventorySummary{},
			Sandboxes:       []Sandbox{},
			Warnings:        warnings,
		}, nil
	}

	stats, statWarnings := s.collectStats(ctx, containers)
	warnings = append(warnings, statWarnings...)

	inventory := s.buildInventory(ctx, containers, networks, volumes, stats)
	inventory.DockerAvailable = true
	inventory.UpdatedAt = s.now()
	inventory.Warnings = compactStrings(warnings)
	return inventory, nil
}

func (s *Service) buildInventory(ctx context.Context, containers []dockerContainer, networks []dockerNetwork, volumes []dockerVolume, stats map[string]ResourceUsage) *Inventory {
	groups := map[string]*Sandbox{}

	for _, item := range containers {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "container", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		container := Container{
			ID:        item.ID,
			Name:      item.Name,
			Image:     item.Image,
			State:     item.State,
			Status:    item.State,
			Ports:     append([]string{}, item.Ports...),
			StartedAt: item.StartedAt,
			ExitCode:  item.ExitCode,
			Usage:     lookupUsage(stats, item.ID, item.Name),
		}
		sandbox.Containers = append(sandbox.Containers, container)
		addUsage(&sandbox.ResourceUsage, container.Usage)
	}

	for _, item := range networks {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "network", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		sandbox.Networks = append(sandbox.Networks, Network{
			ID:     item.ID,
			Name:   item.Name,
			Driver: item.Driver,
			Scope:  item.Scope,
		})
	}

	for _, item := range volumes {
		sandbox := ensureSandbox(groups, sandboxKey(item.Labels, "volume", item.Name))
		hydrateSandbox(sandbox, item.Labels)
		sandbox.Volumes = append(sandbox.Volumes, Volume{
			Name:       item.Name,
			Driver:     item.Driver,
			Mountpoint: item.Mountpoint,
		})
	}

	sandboxes := make([]Sandbox, 0, len(groups))
	for _, sandbox := range groups {
		finalizeSandbox(ctx, s.checker, s.now(), sandbox)
		sandboxes = append(sandboxes, *sandbox)
	}

	sort.Slice(sandboxes, func(i, j int) bool {
		leftRank := sandboxStatusRank(sandboxes[i].Status)
		rightRank := sandboxStatusRank(sandboxes[j].Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftTime := zeroTime(sandboxes[i].StartedAt)
		rightTime := zeroTime(sandboxes[j].StartedAt)
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return sandboxes[i].SandboxID < sandboxes[j].SandboxID
	})

	inventory := &Inventory{
		Sandboxes: sandboxes,
	}
	for index := range sandboxes {
		item := sandboxes[index]
		inventory.Summary.ActiveSandboxes++
		if item.IsZombie {
			inventory.Summary.ZombieSandboxes++
		}
		inventory.Summary.Containers += len(item.Containers)
		inventory.Summary.Networks += len(item.Networks)
		inventory.Summary.Volumes += len(item.Volumes)
		inventory.Summary.TotalCPUPercent += item.ResourceUsage.CPUPercent
		inventory.Summary.TotalMemoryBytes += item.ResourceUsage.MemoryBytes
	}

	return inventory
}

func (s *Service) collectStats(ctx context.Context, containers []dockerContainer) (map[string]ResourceUsage, []string) {
	runningIDs := []string{}
	for _, item := range containers {
		if item.State == "running" {
			runningIDs = append(runningIDs, item.ID)
		}
	}
	if len(runningIDs) == 0 {
		return map[string]ResourceUsage{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, runningIDs...)...)
	if err != nil {
		return map[string]ResourceUsage{}, []string{fmt.Sprintf("Live CPU and memory stats are unavailable: %s", humanizeDockerError(err))}
	}

	stats := map[string]ResourceUsage{}
	for _, line := range splitLines(output) {
		var row struct {
			ID       string `json:"ID"`
			Name     string `json:"Name"`
			CPUPerc  string `json:"CPUPerc"`
			MemUsage string `json:"MemUsage"`
			MemPerc  string `json:"MemPerc"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}

		used, limit := parseMemoryUsage(row.MemUsage)
		usage := ResourceUsage{
			CPUPercent:       parsePercent(row.CPUPerc),
			MemoryBytes:      used,
			MemoryLimitBytes: limit,
			MemoryPercent:    parsePercent(row.MemPerc),
		}
		if row.ID != "" {
			stats[row.ID] = usage
		}
		if row.Name != "" {
			stats[row.Name] = usage
		}
	}

	return stats, nil
}

func snapshotSignature(inventory *Inventory) string {
	if inventory == nil {
		return ""
	}

	clone := *inventory
	clone.UpdatedAt = time.Time{}
	data, err := json.Marshal(clone)
	if err != nil {
		return ""
	}
	return string(data)
}
