package api

import "time"

type ErrorResponse struct {
	Error string `json:"error"`
}

type RunStatus string

const (
	RunPending  RunStatus = "pending"
	RunRunning  RunStatus = "running"
	RunSuccess  RunStatus = "success"
	RunFailure  RunStatus = "failure"
	RunCanceled RunStatus = "canceled"
	RunError    RunStatus = "error"
)

type User struct {
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

type Org struct {
	OrgID     string    `json:"org_id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
	Org   *Org   `json:"org"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CatalogPackage struct {
	PackageID      string    `json:"package_id"`
	OrgID          string    `json:"org_id"`
	RegistryID     string    `json:"registry_id"`
	RegistryKind   string    `json:"registry_kind"`
	Name           string    `json:"name"`
	DisplayName    string    `json:"display_name"`
	Description    string    `json:"description"`
	Publisher      string    `json:"publisher"`
	ImageRef       string    `json:"image_ref"`
	Version        string    `json:"version"`
	Tags           []string  `json:"tags"`
	Profiles       []string  `json:"profiles,omitempty"`
	DefaultProfile string    `json:"default_profile,omitempty"`
	ServiceCount   int       `json:"service_count,omitempty"`
	MockCount      int       `json:"mock_count,omitempty"`
	TestCount      int       `json:"test_count,omitempty"`
	ContractCount  int       `json:"contract_count,omitempty"`
	Enabled        bool      `json:"enabled"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Registry struct {
	RegistryID            string    `json:"registry_id"`
	OrgID                 string    `json:"org_id"`
	Kind                  string    `json:"kind"`
	Name                  string    `json:"name"`
	URL                   string    `json:"url"`
	InsecureSkipTLSVerify bool      `json:"insecure_skip_tls_verify,omitempty"`
	Username              string    `json:"username,omitempty"`
	Password              string    `json:"password,omitempty"`
	BearerToken           string    `json:"bearer_token,omitempty"`
	TLSCAData             string    `json:"tls_ca_data,omitempty"`
	TLSCertData           string    `json:"tls_cert_data,omitempty"`
	TLSKeyData            string    `json:"tls_key_data,omitempty"`
	HasPassword           bool      `json:"has_password,omitempty"`
	HasBearerToken        bool      `json:"has_bearer_token,omitempty"`
	HasTLSCAData          bool      `json:"has_tls_ca_data,omitempty"`
	HasTLSCertData        bool      `json:"has_tls_cert_data,omitempty"`
	HasTLSKeyData         bool      `json:"has_tls_key_data,omitempty"`
	Enabled               bool      `json:"enabled"`
	CreatedAt             time.Time `json:"created_at"`
}

type CatalogListResponse struct {
	Packages   []*CatalogPackage `json:"packages"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

type Run struct {
	RunID      string     `json:"run_id"`
	OrgID      string     `json:"org_id"`
	PackageID  string     `json:"package_id"`
	ImageRef   string     `json:"image_ref"`
	Profile    string     `json:"profile"`
	AgentID    string     `json:"agent_id"`
	Status     RunStatus  `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Step struct {
	StepID     string     `json:"step_id"`
	RunID      string     `json:"run_id"`
	Name       string     `json:"name"`
	Position   int        `json:"position"`
	Type       string     `json:"type"`
	Status     RunStatus  `json:"status"`
	ExitCode   int        `json:"exit_code"`
	Error      string     `json:"error"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type LogEntry struct {
	LogID   string `json:"log_id"`
	RunID   string `json:"run_id"`
	StepID  string `json:"step_id"`
	Line    int    `json:"line"`
	Data    string `json:"data"`
	Time    int64  `json:"time"`
	Type    int    `json:"type"`
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id"`
}

type RunListResponse struct {
	Runs  []*Run `json:"runs"`
	Total int64  `json:"total"`
	Page  int    `json:"page"`
}

type CreateRunRequest struct {
	PackageID string `json:"package_id"`
	Profile   string `json:"profile,omitempty"`
}

type Agent struct {
	AgentID           string            `json:"agent_id"`
	OrgID             string            `json:"org_id"`
	Name              string            `json:"name"`
	RuntimeTargetID   string            `json:"runtime_target_id"`
	DesiredBackend    string            `json:"desired_backend"`
	DesiredPlatform   string            `json:"desired_platform"`
	DesiredTargetName string            `json:"desired_target_name"`
	DesiredTargetURL  string            `json:"desired_target_url"`
	Platform          string            `json:"platform"`
	Backend           string            `json:"backend"`
	TargetName        string            `json:"target_name"`
	TargetURL         string            `json:"target_url"`
	Capacity          int               `json:"capacity"`
	Version           string            `json:"version"`
	Labels            map[string]string `json:"labels"`
	LastContact       time.Time         `json:"last_contact"`
	LastWork          *time.Time        `json:"last_work,omitempty"`
	NoSchedule        bool              `json:"no_schedule"`
	CreatedAt         time.Time         `json:"created_at"`
}

type CreateAgentRequest struct {
	Name            string            `json:"name"`
	RuntimeTargetID string            `json:"runtime_target_id"`
	Capacity        int               `json:"capacity"`
	Labels          map[string]string `json:"labels"`
	NoSchedule      bool              `json:"no_schedule"`
}

type UpdateAgentRequest struct {
	Name            *string           `json:"name,omitempty"`
	RuntimeTargetID *string           `json:"runtime_target_id,omitempty"`
	NoSchedule      *bool             `json:"no_schedule,omitempty"`
	Capacity        *int              `json:"capacity,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
}

type CreateAgentResponse struct {
	Agent *Agent `json:"agent"`
	Token string `json:"token"`
}

type RuntimeTarget struct {
	RuntimeTargetID          string            `json:"runtime_target_id"`
	OrgID                    string            `json:"org_id"`
	Name                     string            `json:"name"`
	Backend                  string            `json:"backend"`
	Platform                 string            `json:"platform"`
	EndpointURL              string            `json:"endpoint_url"`
	Namespace                string            `json:"namespace"`
	RunnerBackend            string            `json:"runner_backend,omitempty"`
	WorkerBootstrapSupported bool              `json:"worker_bootstrap_supported,omitempty"`
	WorkerBootstrapReason    string            `json:"worker_bootstrap_reason,omitempty"`
	InsecureSkipTLSVerify    bool              `json:"insecure_skip_tls_verify,omitempty"`
	Username                 string            `json:"username,omitempty"`
	Password                 string            `json:"password,omitempty"`
	BearerToken              string            `json:"bearer_token,omitempty"`
	TLSCAData                string            `json:"tls_ca_data,omitempty"`
	TLSCertData              string            `json:"tls_cert_data,omitempty"`
	TLSKeyData               string            `json:"tls_key_data,omitempty"`
	HasPassword              bool              `json:"has_password,omitempty"`
	HasBearerToken           bool              `json:"has_bearer_token,omitempty"`
	HasTLSCAData             bool              `json:"has_tls_ca_data,omitempty"`
	HasTLSCertData           bool              `json:"has_tls_cert_data,omitempty"`
	HasTLSKeyData            bool              `json:"has_tls_key_data,omitempty"`
	Labels                   map[string]string `json:"labels"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

type AgentRegisterRequest struct {
	Name       string            `json:"name"`
	Platform   string            `json:"platform"`
	Backend    string            `json:"backend"`
	TargetName string            `json:"target_name"`
	TargetURL  string            `json:"target_url"`
	Capacity   int               `json:"capacity"`
	Version    string            `json:"version"`
	Labels     map[string]string `json:"labels"`
}

type AgentRegisterResponse struct {
	AgentID string `json:"agent_id"`
}

type AgentBootstrapResponse struct {
	AgentID                  string         `json:"agent_id"`
	AgentName                string         `json:"agent_name"`
	Capacity                 int            `json:"capacity"`
	NoSchedule               bool           `json:"no_schedule"`
	RunnerBackend            string         `json:"runner_backend"`
	RuntimeTarget            *RuntimeTarget `json:"runtime_target,omitempty"`
	WorkerBootstrapSupported bool           `json:"worker_bootstrap_supported"`
	WorkerBootstrapReason    string         `json:"worker_bootstrap_reason,omitempty"`
}

type WaitRunResponse struct {
	Canceled bool      `json:"canceled"`
	Status   RunStatus `json:"status"`
}

type UpdateRunRequest struct {
	Status RunStatus `json:"status"`
}

type CreateStepRequest struct {
	Name string `json:"name"`
}

type UpdateStepRequest struct {
	Status   RunStatus `json:"status"`
	ExitCode int       `json:"exit_code"`
	Error    string    `json:"error"`
}

type StepLogLine struct {
	Line    int    `json:"line"`
	Data    string `json:"data"`
	Time    int64  `json:"time"`
	Type    int    `json:"type"`
	TraceID string `json:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty"`
}
