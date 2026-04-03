package platform

import (
	"errors"
	"strings"
)

func normalize(settings *PlatformSettings) {
	if settings.Mode == "" {
		settings.Mode = "local"
	}
	if settings.Description == "" {
		settings.Description = defaultPlatformDescription
	}
	if settings.Agents == nil {
		settings.Agents = []ExecutionAgent{}
	}
	if settings.Registries == nil {
		settings.Registries = []OCIRegistry{}
	}
	if settings.Secrets.GlobalOverrides == nil {
		settings.Secrets.GlobalOverrides = []GlobalOverride{}
	}

	for index := range settings.Agents {
		settings.Agents[index].AgentID = strings.TrimSpace(settings.Agents[index].AgentID)
		settings.Agents[index].Name = strings.TrimSpace(settings.Agents[index].Name)
		settings.Agents[index].Type = strings.TrimSpace(settings.Agents[index].Type)
		settings.Agents[index].Status = strings.TrimSpace(settings.Agents[index].Status)
		settings.Agents[index].RoutingTags = compact(settings.Agents[index].RoutingTags)
		normalizeAPISIXSidecar(&settings.Agents[index].APISIXSidecar)
	}

	for index := range settings.Registries {
		settings.Registries[index].RegistryID = strings.TrimSpace(settings.Registries[index].RegistryID)
		settings.Registries[index].Name = strings.TrimSpace(settings.Registries[index].Name)
		settings.Registries[index].Provider = strings.TrimSpace(settings.Registries[index].Provider)
		settings.Registries[index].RegistryURL = strings.TrimSpace(settings.Registries[index].RegistryURL)
		settings.Registries[index].Username = strings.TrimSpace(settings.Registries[index].Username)
		settings.Registries[index].RepositoryScope = strings.TrimSpace(settings.Registries[index].RepositoryScope)
		settings.Registries[index].Region = strings.TrimSpace(settings.Registries[index].Region)
		settings.Registries[index].SyncStatus = strings.TrimSpace(settings.Registries[index].SyncStatus)
	}

	for index := range settings.Secrets.GlobalOverrides {
		settings.Secrets.GlobalOverrides[index].Key = strings.TrimSpace(settings.Secrets.GlobalOverrides[index].Key)
		settings.Secrets.GlobalOverrides[index].Description = strings.TrimSpace(settings.Secrets.GlobalOverrides[index].Description)
	}
}

func validate(settings *PlatformSettings) error {
	if len(settings.Agents) == 0 {
		return errors.New("Add at least one execution agent.")
	}
	if len(settings.Registries) == 0 {
		return errors.New("Add at least one OCI registry.")
	}

	for _, agent := range settings.Agents {
		if agent.AgentID == "" || agent.Name == "" || agent.Type == "" {
			return errors.New("Each execution agent needs an ID, name, and type.")
		}
		if strings.TrimSpace(agent.APISIXSidecar.Image) == "" {
			return errors.New("Each execution agent needs an APISIX sidecar image.")
		}
		if strings.TrimSpace(agent.APISIXSidecar.ConfigMountPath) == "" {
			return errors.New("Each execution agent needs an APISIX config mount path.")
		}
		if agent.APISIXSidecar.ListenPort <= 0 || agent.APISIXSidecar.AdminPort <= 0 {
			return errors.New("Each execution agent needs positive APISIX listen and admin ports.")
		}
	}

	for _, registry := range settings.Registries {
		if registry.RegistryID == "" || registry.Name == "" || registry.Provider == "" || registry.RegistryURL == "" {
			return errors.New("Each OCI registry needs an ID, name, provider, and registry URL.")
		}
	}

	for _, override := range settings.Secrets.GlobalOverrides {
		if override.Key == "" {
			return errors.New("Global overrides must have a key.")
		}
	}

	return nil
}

func compact(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func normalizeAPISIXSidecar(sidecar *APISIXSidecarConfig) {
	defaults := defaultAPISIXSidecar()

	sidecar.Image = strings.TrimSpace(sidecar.Image)
	sidecar.ConfigMountPath = strings.TrimSpace(sidecar.ConfigMountPath)
	sidecar.Capabilities = compact(sidecar.Capabilities)

	if sidecar.Image == "" {
		sidecar.Image = defaults.Image
	}
	if sidecar.ConfigMountPath == "" {
		sidecar.ConfigMountPath = defaults.ConfigMountPath
	}
	if sidecar.ListenPort == 0 {
		sidecar.ListenPort = defaults.ListenPort
	}
	if sidecar.AdminPort == 0 {
		sidecar.AdminPort = defaults.AdminPort
	}
	if len(sidecar.Capabilities) == 0 {
		sidecar.Capabilities = append([]string{}, defaults.Capabilities...)
	}
}
