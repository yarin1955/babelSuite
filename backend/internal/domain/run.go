package domain

import "time"

type RunStatus string

const (
	RunPending  RunStatus = "pending"
	RunRunning  RunStatus = "running"
	RunSuccess  RunStatus = "success"
	RunFailure  RunStatus = "failure"
	RunCanceled RunStatus = "canceled"
	RunError    RunStatus = "error"
)

type Run struct {
	RunID      string     `json:"run_id"      bson:"run_id"`
	OrgID      string     `json:"org_id"      bson:"org_id"`
	PackageID  string     `json:"package_id"  bson:"package_id"`
	ImageRef   string     `json:"image_ref"   bson:"image_ref"`
	AgentID    string     `json:"agent_id"    bson:"agent_id"`
	Status     RunStatus  `json:"status"      bson:"status"`
	StartedAt  *time.Time `json:"started_at"  bson:"started_at"`
	FinishedAt *time.Time `json:"finished_at" bson:"finished_at"`
	CreatedAt  time.Time  `json:"created_at"  bson:"created_at"`
}

type Step struct {
	StepID     string     `json:"step_id"     bson:"step_id"`
	RunID      string     `json:"run_id"      bson:"run_id"`
	Name       string     `json:"name"        bson:"name"`
	Position   int        `json:"position"    bson:"position"`
	Status     RunStatus  `json:"status"      bson:"status"`
	ExitCode   int        `json:"exit_code"   bson:"exit_code"`
	Error      string     `json:"error"       bson:"error"`
	StartedAt  *time.Time `json:"started_at"  bson:"started_at"`
	FinishedAt *time.Time `json:"finished_at" bson:"finished_at"`
}

// LogEntryType mirrors the reference log entry types.
type LogEntryType int

const (
	LogStdout   LogEntryType = 0
	LogStderr   LogEntryType = 1
	LogExitCode LogEntryType = 2
)

type LogEntry struct {
	LogID  string       `json:"log_id"  bson:"log_id"`
	RunID  string       `json:"run_id"  bson:"run_id"`
	StepID string       `json:"step_id" bson:"step_id"`
	Line   int          `json:"line"    bson:"line"`
	Data   string       `json:"data"    bson:"data"`
	Time   int64        `json:"time"    bson:"time"` // unix milliseconds
	Type   LogEntryType `json:"type"    bson:"type"`
}
