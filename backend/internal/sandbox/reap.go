package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func (s *Service) ReapSandbox(ctx context.Context, sandboxID string) (*ReapResult, error) {
	sandboxID = strings.TrimSpace(sandboxID)
	if sandboxID == "" {
		return nil, ErrNotFound
	}

	containers, networks, volumes, dockerAvailable, _, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}
	if !dockerAvailable {
		return nil, ErrDockerUnavailable
	}

	inventory := s.buildInventory(ctx, containers, networks, volumes, nil)
	var target *Sandbox
	for index := range inventory.Sandboxes {
		if inventory.Sandboxes[index].SandboxID == sandboxID {
			target = &inventory.Sandboxes[index]
			break
		}
	}
	if target == nil {
		return nil, ErrNotFound
	}

	result, err := s.reapResources(ctx, "sandbox", sandboxID, target.Containers, target.Networks, target.Volumes)
	if err != nil {
		return nil, err
	}

	_ = s.publishLatestSnapshot(ctx, "sandbox-reap", false)
	return result, nil
}

func (s *Service) ReapAll(ctx context.Context) (*ReapResult, error) {
	containers, networks, volumes, dockerAvailable, _, err := s.collectResources(ctx)
	if err != nil {
		return nil, err
	}
	if !dockerAvailable {
		return nil, ErrDockerUnavailable
	}

	allContainers := make([]Container, 0, len(containers))
	for _, item := range containers {
		allContainers = append(allContainers, Container{ID: item.ID, Name: item.Name})
	}
	allNetworks := make([]Network, 0, len(networks))
	for _, item := range networks {
		allNetworks = append(allNetworks, Network{ID: item.ID, Name: item.Name})
	}
	allVolumes := make([]Volume, 0, len(volumes))
	for _, item := range volumes {
		allVolumes = append(allVolumes, Volume{Name: item.Name})
	}

	result, err := s.reapResources(ctx, "global", "all-managed-sandboxes", allContainers, allNetworks, allVolumes)
	if err != nil {
		return nil, err
	}

	_ = s.publishLatestSnapshot(ctx, "global-reap", false)
	return result, nil
}

func (s *Service) reapResources(ctx context.Context, scope, target string, containers []Container, networks []Network, volumes []Volume) (*ReapResult, error) {
	result := &ReapResult{
		Scope:             scope,
		Target:            target,
		RemovedContainers: len(containers),
		RemovedNetworks:   len(networks),
		RemovedVolumes:    len(volumes),
		Warnings:          []string{},
	}

	if len(containers) > 0 {
		ids := make([]string, 0, len(containers))
		for _, item := range containers {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			if err := s.runDocker(ctx, append([]string{"rm", "-f"}, ids...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Container cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	if len(networks) > 0 {
		ids := make([]string, 0, len(networks))
		for _, item := range networks {
			if item.ID != "" {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) > 0 {
			if err := s.runDocker(ctx, append([]string{"network", "rm"}, ids...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Network cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	if len(volumes) > 0 {
		names := make([]string, 0, len(volumes))
		for _, item := range volumes {
			if item.Name != "" {
				names = append(names, item.Name)
			}
		}
		if len(names) > 0 {
			if err := s.runDocker(ctx, append([]string{"volume", "rm", "-f"}, names...)...); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Volume cleanup reported an error: %s", humanizeDockerError(err)))
			}
		}
	}

	result.Warnings = compactStrings(result.Warnings)
	return result, nil
}

func (s *Service) collectResources(ctx context.Context) ([]dockerContainer, []dockerNetwork, []dockerVolume, bool, []string, error) {
	warnings := []string{}

	containers, err := s.listContainers(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Containers could not be inspected: %s", humanizeDockerError(err)))
	}

	networks, err := s.listNetworks(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Networks could not be inspected: %s", humanizeDockerError(err)))
	}

	volumes, err := s.listVolumes(ctx)
	if err != nil {
		if errors.Is(err, ErrDockerUnavailable) {
			return nil, nil, nil, false, []string{humanizeDockerError(err)}, nil
		}
		warnings = append(warnings, fmt.Sprintf("Volumes could not be inspected: %s", humanizeDockerError(err)))
	}

	return containers, networks, volumes, true, compactStrings(warnings), nil
}
