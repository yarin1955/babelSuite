package environments

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func (s *Service) listContainers(ctx context.Context) ([]dockerContainer, error) {
	idsOutput, err := s.runDockerOutput(ctx, "ps", "-aq", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	ids := splitLines(idsOutput)
	if len(ids) == 0 {
		return []dockerContainer{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerContainerInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	containers := make([]dockerContainer, 0, len(raw))
	for _, item := range raw {
		container := dockerContainer{
			ID:       item.ID,
			Name:     strings.TrimPrefix(item.Name, "/"),
			Image:    item.Config.Image,
			Labels:   cloneLabels(item.Config.Labels),
			State:    strings.TrimSpace(item.State.Status),
			ExitCode: item.State.ExitCode,
			Ports:    formatPorts(item.NetworkSettings.Ports),
		}
		if startedAt := parseDockerTime(item.State.StartedAt); startedAt != nil {
			container.StartedAt = startedAt
		}
		containers = append(containers, container)
	}

	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	return containers, nil
}

func (s *Service) listNetworks(ctx context.Context) ([]dockerNetwork, error) {
	idsOutput, err := s.runDockerOutput(ctx, "network", "ls", "-q", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	ids := splitLines(idsOutput)
	if len(ids) == 0 {
		return []dockerNetwork{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"network", "inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerNetworkInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	networks := make([]dockerNetwork, 0, len(raw))
	for _, item := range raw {
		networks = append(networks, dockerNetwork{
			ID:     item.ID,
			Name:   item.Name,
			Driver: item.Driver,
			Scope:  item.Scope,
			Labels: cloneLabels(item.Labels),
		})
	}

	sort.Slice(networks, func(i, j int) bool { return networks[i].Name < networks[j].Name })
	return networks, nil
}

func (s *Service) listVolumes(ctx context.Context) ([]dockerVolume, error) {
	namesOutput, err := s.runDockerOutput(ctx, "volume", "ls", "-q", "--filter", "label="+managedLabelFilter)
	if err != nil {
		return nil, err
	}
	names := splitLines(namesOutput)
	if len(names) == 0 {
		return []dockerVolume{}, nil
	}

	output, err := s.runDockerOutput(ctx, append([]string{"volume", "inspect"}, names...)...)
	if err != nil {
		return nil, err
	}

	var raw []dockerVolumeInspect
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}

	volumes := make([]dockerVolume, 0, len(raw))
	for _, item := range raw {
		volumes = append(volumes, dockerVolume{
			Name:       item.Name,
			Driver:     item.Driver,
			Mountpoint: item.Mountpoint,
			Labels:     cloneLabels(item.Labels),
		})
	}

	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	return volumes, nil
}

func (s *Service) runDocker(ctx context.Context, args ...string) error {
	_, err := s.runDockerOutput(ctx, args...)
	return err
}

func (s *Service) runDockerOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	out, err := s.runner.Run(ctx, "docker", args...)
	if err != nil {
		return nil, classifyDockerError(err, out)
	}
	return out, nil
}
