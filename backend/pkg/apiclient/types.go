package apiclient

import "time"

type User struct {
	UserID      string    `json:"userId"`
	WorkspaceID string    `json:"workspaceId"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	FullName    string    `json:"fullName"`
	IsAdmin     bool      `json:"isAdmin"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Workspace struct {
	WorkspaceID string    `json:"workspaceId"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AuthResponse struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	Workspace Workspace `json:"workspace"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type CatalogPackage struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Repository  string   `json:"repository"`
	Owner       string   `json:"owner"`
	Provider    string   `json:"provider"`
	Version     string   `json:"version"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Modules     []string `json:"modules"`
	Status      string   `json:"status"`
	Score       int      `json:"score"`
	PullCommand string   `json:"pullCommand"`
	ForkCommand string   `json:"forkCommand"`
	Inspectable bool     `json:"inspectable"`
	Starred     bool     `json:"starred"`
}

type ProfileOption struct {
	FileName    string `json:"fileName"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type SuiteSourceFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type SuiteDefinition struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Repository  string            `json:"repository"`
	Owner       string            `json:"owner"`
	Provider    string            `json:"provider"`
	Version     string            `json:"version"`
	Tags        []string          `json:"tags"`
	Description string            `json:"description"`
	Modules     []string          `json:"modules"`
	Status      string            `json:"status"`
	SuiteStar   string            `json:"suiteStar"`
	Profiles    []ProfileOption   `json:"profiles"`
	SourceFiles []SuiteSourceFile `json:"sourceFiles"`
}

type ProfileSecretReference struct {
	Key      string `json:"key"`
	Provider string `json:"provider"`
	Ref      string `json:"ref"`
}

type ProfileRecord struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	FileName    string                   `json:"fileName"`
	Description string                   `json:"description"`
	Scope       string                   `json:"scope"`
	Yaml        string                   `json:"yaml"`
	SecretRefs  []ProfileSecretReference `json:"secretRefs"`
	Default     bool                     `json:"default"`
	ExtendsID   string                   `json:"extendsId,omitempty"`
	Launchable  bool                     `json:"launchable"`
	UpdatedAt   time.Time                `json:"updatedAt"`
}

type SuiteProfilesResponse struct {
	SuiteID                string          `json:"suiteId"`
	SuiteTitle             string          `json:"suiteTitle"`
	SuiteDescription       string          `json:"suiteDescription"`
	Repository             string          `json:"repository"`
	DefaultProfileID       string          `json:"defaultProfileId"`
	DefaultProfileFileName string          `json:"defaultProfileFileName"`
	Profiles               []ProfileRecord `json:"profiles"`
}

type LaunchSuite struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Repository  string          `json:"repository"`
	Description string          `json:"description"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	Profiles    []ProfileOption `json:"profiles"`
}

type ExecutionSummary struct {
	ID         string    `json:"id"`
	SuiteID    string    `json:"suiteId"`
	SuiteTitle string    `json:"suiteTitle"`
	Profile    string    `json:"profile"`
	Trigger    string    `json:"trigger"`
	Status     string    `json:"status"`
	Duration   string    `json:"duration"`
	StartedAt  time.Time `json:"startedAt"`
}

type ExecutionEvent struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	Level     string `json:"level"`
}

type ExecutionSuite struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Repository string          `json:"repository"`
	SuiteStar  string          `json:"suiteStar"`
	Profiles   []ProfileOption `json:"profiles"`
}

type ExecutionRecord struct {
	ID        string           `json:"id"`
	Suite     ExecutionSuite   `json:"suite"`
	Profile   string           `json:"profile"`
	Trigger   string           `json:"trigger"`
	Status    string           `json:"status"`
	Duration  string           `json:"duration"`
	StartedAt time.Time        `json:"startedAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Author    string           `json:"author"`
	Commit    string           `json:"commit"`
	Branch    string           `json:"branch"`
	Message   string           `json:"message"`
	Events    []ExecutionEvent `json:"events"`
}

type SandboxUsage struct {
	CPUPercent       float64 `json:"cpuPercent"`
	MemoryBytes      int64   `json:"memoryBytes"`
	MemoryLimitBytes int64   `json:"memoryLimitBytes"`
	MemoryPercent    float64 `json:"memoryPercent"`
}

type SandboxContainer struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Image     string       `json:"image"`
	State     string       `json:"state"`
	Status    string       `json:"status"`
	Ports     []string     `json:"ports"`
	StartedAt *time.Time   `json:"startedAt,omitempty"`
	ExitCode  *int         `json:"exitCode,omitempty"`
	Usage     SandboxUsage `json:"usage"`
}

type SandboxNetwork struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

type SandboxVolume struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
}

type SandboxRecord struct {
	SandboxID         string             `json:"sandboxId"`
	RunID             string             `json:"runId"`
	Suite             string             `json:"suite"`
	Owner             string             `json:"owner"`
	Profile           string             `json:"profile"`
	Status            string             `json:"status"`
	Summary           string             `json:"summary"`
	StartedAt         *time.Time         `json:"startedAt,omitempty"`
	LastHeartbeatAt   *time.Time         `json:"lastHeartbeatAt,omitempty"`
	OrchestratorPID   int                `json:"orchestratorPid,omitempty"`
	OrchestratorState string             `json:"orchestratorState"`
	IsZombie          bool               `json:"isZombie"`
	CanReap           bool               `json:"canReap"`
	ResourceUsage     SandboxUsage       `json:"resourceUsage"`
	Containers        []SandboxContainer `json:"containers"`
	Networks          []SandboxNetwork   `json:"networks"`
	Volumes           []SandboxVolume    `json:"volumes"`
	Warnings          []string           `json:"warnings"`
}

type SandboxesResponse struct {
	DockerAvailable bool            `json:"dockerAvailable"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Sandboxes       []SandboxRecord `json:"sandboxes"`
	Warnings        []string        `json:"warnings"`
}

type ReapResult struct {
	Scope             string   `json:"scope"`
	Target            string   `json:"target"`
	RemovedContainers int      `json:"removedContainers"`
	RemovedNetworks   int      `json:"removedNetworks"`
	RemovedVolumes    int      `json:"removedVolumes"`
	Warnings          []string `json:"warnings"`
}
