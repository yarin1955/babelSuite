package engine

import "time"

type StepState struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Status    string   `json:"status"`
	DependsOn []string `json:"dependsOn"`
}

type ExecutionState struct {
	ID            string      `json:"id"`
	SuiteID       string      `json:"suiteId"`
	SuiteTitle    string      `json:"suiteTitle"`
	Profile       string      `json:"profile"`
	BackendID     string      `json:"backendId"`
	Backend       string      `json:"backend"`
	Trigger       string      `json:"trigger"`
	Status        string      `json:"status"`
	Duration      string      `json:"duration"`
	StartedAt     time.Time   `json:"startedAt"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	TotalSteps    int         `json:"totalSteps"`
	RunningSteps  int         `json:"runningSteps"`
	HealthySteps  int         `json:"healthySteps"`
	FailedSteps   int         `json:"failedSteps"`
	SkippedSteps  int         `json:"skippedSteps,omitempty"`
	PendingSteps  int         `json:"pendingSteps"`
	ProgressRatio float64     `json:"progressRatio"`
	Steps         []StepState `json:"steps"`
}

type State struct {
	UpdatedAt  time.Time                 `json:"updatedAt"`
	Executions map[string]ExecutionState `json:"executions"`
	Order      []string                  `json:"order"`
}
