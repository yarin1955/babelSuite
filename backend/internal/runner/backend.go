package runner

import "strings"

const AutoBackend = "auto"

func normalizeBackendConfig(config BackendConfig, fallbackID, fallbackLabel, fallbackKind string) BackendConfig {
	config.ID = firstNonEmpty(config.ID, fallbackID)
	config.Label = firstNonEmpty(config.Label, fallbackLabel)
	config.Kind = firstNonEmpty(config.Kind, fallbackKind)
	return config
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
