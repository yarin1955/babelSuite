package environments

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/babelsuite/babelsuite/internal/eventstream"
)

var (
	ErrNotFound          = errors.New("sandbox not found")
	ErrDockerUnavailable = errors.New("docker unavailable")
)

const managedLabelFilter = "babelsuite.managed=true"
const inventoryStreamKey = "inventory"

type Manager interface {
	Snapshot(ctx context.Context) (*Inventory, error)
	ReapSandbox(ctx context.Context, sandboxID string) (*ReapResult, error)
	ReapAll(ctx context.Context) (*ReapResult, error)
	SubscribeEvents(ctx context.Context, since int) (<-chan StreamEvent, error)
}

type Inventory struct {
	DockerAvailable bool             `json:"dockerAvailable"`
	UpdatedAt       time.Time        `json:"updatedAt"`
	Summary         InventorySummary `json:"summary"`
	Sandboxes       []Sandbox        `json:"sandboxes"`
	Warnings        []string         `json:"warnings"`
}

type InventorySummary struct {
	ActiveSandboxes  int     `json:"activeSandboxes"`
	ZombieSandboxes  int     `json:"zombieSandboxes"`
	Containers       int     `json:"containers"`
	Networks         int     `json:"networks"`
	Volumes          int     `json:"volumes"`
	TotalCPUPercent  float64 `json:"totalCpuPercent"`
	TotalMemoryBytes int64   `json:"totalMemoryBytes"`
}

type Sandbox struct {
	SandboxID         string        `json:"sandboxId"`
	RunID             string        `json:"runId"`
	Suite             string        `json:"suite"`
	Owner             string        `json:"owner"`
	Profile           string        `json:"profile"`
	Status            string        `json:"status"`
	Summary           string        `json:"summary"`
	StartedAt         *time.Time    `json:"startedAt,omitempty"`
	LastHeartbeatAt   *time.Time    `json:"lastHeartbeatAt,omitempty"`
	OrchestratorPID   int           `json:"orchestratorPid,omitempty"`
	OrchestratorState string        `json:"orchestratorState"`
	IsZombie          bool          `json:"isZombie"`
	CanReap           bool          `json:"canReap"`
	ResourceUsage     ResourceUsage `json:"resourceUsage"`
	Containers        []Container   `json:"containers"`
	Networks          []Network     `json:"networks"`
	Volumes           []Volume      `json:"volumes"`
	Warnings          []string      `json:"warnings"`
}

type ResourceUsage struct {
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryBytes      int64   `json:"memoryBytes"`
	MemoryLimitBytes int64   `json:"memoryLimitBytes"`
	MemoryPercent    float64 `json:"memoryPercent"`
}

type Container struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Image     string        `json:"image"`
	State     string        `json:"state"`
	Status    string        `json:"status"`
	Ports     []string      `json:"ports"`
	StartedAt *time.Time    `json:"startedAt,omitempty"`
	ExitCode  int           `json:"exitCode,omitempty"`
	Usage     ResourceUsage `json:"usage"`
}

type Network struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

type Volume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

type ReapResult struct {
	Scope             string   `json:"scope"`
	Target            string   `json:"target"`
	RemovedContainers int      `json:"removedContainers"`
	RemovedNetworks   int      `json:"removedNetworks"`
	RemovedVolumes    int      `json:"removedVolumes"`
	Warnings          []string `json:"warnings"`
}

type StreamEvent struct {
	ID       int       `json:"id"`
	Reason   string    `json:"reason"`
	Snapshot Inventory `json:"snapshot"`
}

type streamPayload struct {
	Reason   string
	Snapshot Inventory
}

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type processChecker interface {
	Alive(ctx context.Context, pid int) bool
}

type Service struct {
	runner  commandRunner
	checker processChecker
	now     func() time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	events  *eventstream.Hub[streamPayload]
	stream  struct {
		mu          sync.Mutex
		lastSig     string
		hasSnapshot bool
	}
}

type dockerContainer struct {
	ID        string
	Name      string
	Image     string
	Labels    map[string]string
	State     string
	StartedAt *time.Time
	ExitCode  int
	Ports     []string
}

type dockerNetwork struct {
	ID     string
	Name   string
	Driver string
	Scope  string
	Labels map[string]string
}

type dockerVolume struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
}

type dockerContainerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"`
		ExitCode  int    `json:"ExitCode"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]dockerPortBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

type dockerPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type dockerNetworkInspect struct {
	ID     string            `json:"Id"`
	Name   string            `json:"Name"`
	Driver string            `json:"Driver"`
	Scope  string            `json:"Scope"`
	Labels map[string]string `json:"Labels"`
}

type dockerVolumeInspect struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
}
