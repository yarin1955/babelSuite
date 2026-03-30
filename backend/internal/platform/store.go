package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	Load() (*PlatformSettings, error)
	Save(settings *PlatformSettings) error
	SyncRegistry(registryID string) (*PlatformSettings, error)
}

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Load() (*PlatformSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadUnlocked()
}

func (s *FileStore) Save(settings *PlatformSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked(settings)
}

func (s *FileStore) SyncRegistry(registryID string) (*PlatformSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	for index := range settings.Registries {
		if settings.Registries[index].RegistryID == registryID {
			now := time.Now().UTC()
			settings.Registries[index].LastSyncedAt = &now
			settings.Registries[index].SyncStatus = "Indexed"
			settings.UpdatedAt = now
			if err := s.saveUnlocked(settings); err != nil {
				return nil, err
			}
			return settings, nil
		}
	}

	return nil, ErrNotFound
}

func (s *FileStore) loadUnlocked() (*PlatformSettings, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			settings := DefaultSettings()
			return &settings, nil
		}
		return nil, err
	}

	var settings PlatformSettings
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	normalize(&settings)
	return &settings, nil
}

func (s *FileStore) saveUnlocked(settings *PlatformSettings) error {
	normalize(settings)
	settings.UpdatedAt = time.Now().UTC()

	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return os.WriteFile(s.path, data, 0o644)
}

type PlatformSettings struct {
	Mode        string             `json:"mode" yaml:"mode"`
	Agents      []ExecutionAgent   `json:"agents" yaml:"agents"`
	Registries  []OCIRegistry      `json:"registries" yaml:"registries"`
	Secrets     SecretsConfig      `json:"secrets" yaml:"secrets"`
	UpdatedAt   time.Time          `json:"updatedAt" yaml:"updatedAt"`
	Description string             `json:"description" yaml:"description"`
}

type ExecutionAgent struct {
	AgentID             string   `json:"agentId" yaml:"agentId"`
	Name                string   `json:"name" yaml:"name"`
	Type                string   `json:"type" yaml:"type"`
	Description         string   `json:"description" yaml:"description"`
	Enabled             bool     `json:"enabled" yaml:"enabled"`
	Default             bool     `json:"default" yaml:"default"`
	Status              string   `json:"status" yaml:"status"`
	RoutingTags         []string `json:"routingTags" yaml:"routingTags"`
	DockerSocket        string   `json:"dockerSocket" yaml:"dockerSocket"`
	HostURL             string   `json:"hostUrl" yaml:"hostUrl"`
	TLSCert             string   `json:"tlsCert" yaml:"tlsCert"`
	TLSKey              string   `json:"tlsKey" yaml:"tlsKey"`
	KubeconfigPath      string   `json:"kubeconfigPath" yaml:"kubeconfigPath"`
	TargetNamespace     string   `json:"targetNamespace" yaml:"targetNamespace"`
	ServiceAccountToken string   `json:"serviceAccountToken" yaml:"serviceAccountToken"`
}

type OCIRegistry struct {
	RegistryID      string     `json:"registryId" yaml:"registryId"`
	Name            string     `json:"name" yaml:"name"`
	Provider        string     `json:"provider" yaml:"provider"`
	RegistryURL     string     `json:"registryUrl" yaml:"registryUrl"`
	Username        string     `json:"username" yaml:"username"`
	Secret          string     `json:"secret" yaml:"secret"`
	RepositoryScope string     `json:"repositoryScope" yaml:"repositoryScope"`
	Region          string     `json:"region" yaml:"region"`
	SyncStatus      string     `json:"syncStatus" yaml:"syncStatus"`
	LastSyncedAt    *time.Time `json:"lastSyncedAt,omitempty" yaml:"lastSyncedAt,omitempty"`
}

type SecretsConfig struct {
	Provider        string           `json:"provider" yaml:"provider"`
	VaultAddress    string           `json:"vaultAddress" yaml:"vaultAddress"`
	VaultNamespace  string           `json:"vaultNamespace" yaml:"vaultNamespace"`
	VaultRole       string           `json:"vaultRole" yaml:"vaultRole"`
	AWSRegion       string           `json:"awsRegion" yaml:"awsRegion"`
	SecretPrefix    string           `json:"secretPrefix" yaml:"secretPrefix"`
	GlobalOverrides []GlobalOverride `json:"globalOverrides" yaml:"globalOverrides"`
}

type GlobalOverride struct {
	Key         string `json:"key" yaml:"key"`
	Value       string `json:"value" yaml:"value"`
	Description string `json:"description" yaml:"description"`
	Sensitive   bool   `json:"sensitive" yaml:"sensitive"`
}

func DefaultSettings() PlatformSettings {
	indexedAt := time.Now().UTC().Add(-15 * time.Minute)
	updatedAt := time.Now().UTC()

	return PlatformSettings{
		Mode:        "local",
		Description: "Physical-layer configuration for BabelSuite execution agents, OCI catalog sources, and shared platform credentials.",
		Agents: []ExecutionAgent{
			{
				AgentID:      "local-docker",
				Name:         "Local Docker",
				Type:         "local",
				Description:  "Uses the host machine's native Docker socket with zero extra configuration.",
				Enabled:      true,
				Default:      true,
				Status:       "Ready",
				RoutingTags:  []string{"default", "local"},
				DockerSocket: "/var/run/docker.sock",
			},
			{
				AgentID:      "remote-build",
				Name:         "Remote Docker Pool",
				Type:         "remote-docker",
				Description:  "Connects to a remote Docker daemon for heavier or isolated suite workloads.",
				Enabled:      false,
				Default:      false,
				Status:       "Disconnected",
				RoutingTags:  []string{"high-memory"},
				HostURL:      "tcp://docker.internal.company.com:2376",
				TLSCert:      "vault://kv/platform/remote-docker/cert",
				TLSKey:       "vault://kv/platform/remote-docker/key",
			},
			{
				AgentID:             "kubernetes-burst",
				Name:                "Kubernetes Burst Pool",
				Type:                "kubernetes",
				Description:         "Launches ephemeral pods in Kubernetes when suites need isolated or GPU-enabled compute.",
				Enabled:             false,
				Default:             false,
				Status:              "Standby",
				RoutingTags:         []string{"gpu-enabled", "ci-only"},
				KubeconfigPath:      "/etc/babelsuite/kubeconfig",
				TargetNamespace:     "babelsuite-runs",
				ServiceAccountToken: "vault://kv/platform/k8s/token",
			},
		},
		Registries: []OCIRegistry{
			{
				RegistryID:      "catalog-primary",
				Name:            "Local Zot",
				Provider:        "Zot",
				RegistryURL:     "http://localhost:5000",
				Username:        "",
				Secret:          "",
				RepositoryScope: "*",
				SyncStatus:      "Indexed",
				LastSyncedAt:    &indexedAt,
			},
		},
		Secrets: SecretsConfig{
			Provider:       "vault",
			VaultAddress:   "https://vault.internal.company.com",
			VaultNamespace: "platform",
			VaultRole:      "babelsuite-runners",
			SecretPrefix:   "kv/platform",
			GlobalOverrides: []GlobalOverride{
				{
					Key:         "HTTPS_PROXY",
					Value:       "http://proxy.internal.company.com:8080",
					Description: "Corporate outbound proxy applied to every suite run.",
					Sensitive:   false,
				},
				{
					Key:         "INTERNAL_ROOT_CA",
					Value:       "-----BEGIN CERTIFICATE-----\\nMIIF...\\n-----END CERTIFICATE-----",
					Description: "Root certificate injected into suite containers for internal TLS trust.",
					Sensitive:   true,
				},
			},
		},
		UpdatedAt: updatedAt,
	}
}

func normalize(settings *PlatformSettings) {
	if settings.Mode == "" {
		settings.Mode = "local"
	}
	if settings.Description == "" {
		settings.Description = "Physical-layer configuration for BabelSuite execution agents, OCI catalog sources, and shared platform credentials."
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
