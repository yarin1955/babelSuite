package platform

import (
	"errors"
	"sync"
	"time"
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

type PlatformSettings struct {
	Mode        string           `json:"mode" yaml:"mode"`
	Agents      []ExecutionAgent `json:"agents" yaml:"agents"`
	Registries  []OCIRegistry    `json:"registries" yaml:"registries"`
	Secrets     SecretsConfig    `json:"secrets" yaml:"secrets"`
	UpdatedAt   time.Time        `json:"updatedAt" yaml:"updatedAt"`
	Description string           `json:"description" yaml:"description"`
}

type ExecutionAgent struct {
	AgentID             string              `json:"agentId" yaml:"agentId"`
	Name                string              `json:"name" yaml:"name"`
	Type                string              `json:"type" yaml:"type"`
	Description         string              `json:"description" yaml:"description"`
	Enabled             bool                `json:"enabled" yaml:"enabled"`
	Default             bool                `json:"default" yaml:"default"`
	Status              string              `json:"status" yaml:"status"`
	RegisteredAt        *time.Time          `json:"registeredAt,omitempty" yaml:"registeredAt,omitempty"`
	LastHeartbeatAt     *time.Time          `json:"lastHeartbeatAt,omitempty" yaml:"lastHeartbeatAt,omitempty"`
	RoutingTags         []string            `json:"routingTags" yaml:"routingTags"`
	RuntimeCapabilities []string            `json:"runtimeCapabilities,omitempty" yaml:"runtimeCapabilities,omitempty"`
	DockerSocket        string              `json:"dockerSocket" yaml:"dockerSocket"`
	HostURL             string              `json:"hostUrl" yaml:"hostUrl"`
	TLSCert             string              `json:"tlsCert" yaml:"tlsCert"`
	TLSKey              string              `json:"tlsKey" yaml:"tlsKey"`
	KubeconfigPath      string              `json:"kubeconfigPath" yaml:"kubeconfigPath"`
	TargetNamespace     string              `json:"targetNamespace" yaml:"targetNamespace"`
	ServiceAccountToken string              `json:"serviceAccountToken" yaml:"serviceAccountToken"`
	APISIXSidecar       APISIXSidecarConfig `json:"apisixSidecar" yaml:"apisixSidecar"`
}

type APISIXSidecarConfig struct {
	Image           string   `json:"image" yaml:"image"`
	ConfigMountPath string   `json:"configMountPath" yaml:"configMountPath"`
	ListenPort      int      `json:"listenPort" yaml:"listenPort"`
	AdminPort       int      `json:"adminPort" yaml:"adminPort"`
	Capabilities    []string `json:"capabilities" yaml:"capabilities"`
}

type OCIRegistry struct {
	RegistryID        string     `json:"registryId" yaml:"registryId"`
	Name              string     `json:"name" yaml:"name"`
	Provider          string     `json:"provider" yaml:"provider"`
	RegistryURL       string     `json:"registryUrl" yaml:"registryUrl"`
	Username          string     `json:"username" yaml:"username"`
	Secret            string     `json:"secret" yaml:"secret"`
	RepositoryScope   string     `json:"repositoryScope" yaml:"repositoryScope"`
	Region            string     `json:"region" yaml:"region"`
	AllowLocalNetwork bool       `json:"allowLocalNetwork" yaml:"allowLocalNetwork"`
	SyncStatus        string     `json:"syncStatus" yaml:"syncStatus"`
	LastSyncedAt      *time.Time `json:"lastSyncedAt,omitempty" yaml:"lastSyncedAt,omitempty"`
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
