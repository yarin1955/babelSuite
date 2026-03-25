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

var runStatusPriority = map[RunStatus]int{
	RunError:    0,
	RunCanceled: 1,
	RunRunning:  2,
	RunPending:  3,
	RunFailure:  4,
	RunSuccess:  5,
}

type Run struct {
	RunID      string     `json:"run_id"      bson:"run_id"`
	OrgID      string     `json:"org_id"      bson:"org_id"`
	PackageID  string     `json:"package_id"  bson:"package_id"`
	ImageRef   string     `json:"image_ref"   bson:"image_ref"`
	Profile    string     `json:"profile"     bson:"profile"`
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
	Type       string     `json:"type"        bson:"type"`
	Status     RunStatus  `json:"status"      bson:"status"`
	ExitCode   int        `json:"exit_code"   bson:"exit_code"`
	Error      string     `json:"error"       bson:"error"`
	StartedAt  *time.Time `json:"started_at"  bson:"started_at"`
	FinishedAt *time.Time `json:"finished_at" bson:"finished_at"`
}

type LogEntryType int

const (
	LogStdout   LogEntryType = 0
	LogStderr   LogEntryType = 1
	LogExitCode LogEntryType = 2
)

type LogEntry struct {
	LogID   string       `json:"log_id"   bson:"log_id"`
	RunID   string       `json:"run_id"   bson:"run_id"`
	StepID  string       `json:"step_id"  bson:"step_id"`
	Line    int          `json:"line"     bson:"line"`
	Data    string       `json:"data"     bson:"data"`
	Time    int64        `json:"time"     bson:"time"`
	Type    LogEntryType `json:"type"     bson:"type"`
	TraceID string       `json:"trace_id" bson:"trace_id"`
	SpanID  string       `json:"span_id"  bson:"span_id"`
}

func (s RunStatus) IsTerminal() bool {
	return s == RunSuccess || s == RunFailure || s == RunCanceled || s == RunError
}

func MergeRunStatus(current, next RunStatus) RunStatus {
	if _, ok := runStatusPriority[current]; !ok {
		current = RunPending
	}
	if _, ok := runStatusPriority[next]; !ok {
		next = RunPending
	}
	if runStatusPriority[next] < runStatusPriority[current] {
		return next
	}
	return current
}

func AggregateRunStatus(steps []*Step) RunStatus {
	if len(steps) == 0 {
		return RunPending
	}
	status := RunSuccess
	for _, step := range steps {
		status = MergeRunStatus(status, step.Status)
	}
	return status
}

func ApplyRunStatus(run *Run, status RunStatus, now time.Time) {
	run.Status = status
	if status != RunPending && run.StartedAt == nil {
		ts := now.UTC()
		run.StartedAt = &ts
	}
	if status.IsTerminal() {
		ts := now.UTC()
		run.FinishedAt = &ts
	} else {
		run.FinishedAt = nil
	}
}

func ApplyStepStatus(step *Step, status RunStatus, exitCode int, errText string, now time.Time) {
	step.Status = status
	step.ExitCode = exitCode
	step.Error = errText
	if status != RunPending && step.StartedAt == nil {
		ts := now.UTC()
		step.StartedAt = &ts
	}
	if status.IsTerminal() {
		ts := now.UTC()
		step.FinishedAt = &ts
	} else {
		step.FinishedAt = nil
	}
}

func StepStatusFromResult(exitCode int, errText string) RunStatus {
	switch {
	case errText != "" && exitCode == 0:
		return RunError
	case exitCode != 0:
		return RunFailure
	default:
		return RunSuccess
	}
}
