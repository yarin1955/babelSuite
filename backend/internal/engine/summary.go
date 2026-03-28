package engine

import "time"

type OverviewSummary struct {
	TotalExecutions   int `json:"totalExecutions"`
	BootingExecutions int `json:"bootingExecutions"`
	HealthyExecutions int `json:"healthyExecutions"`
	FailedExecutions  int `json:"failedExecutions"`
	TotalSteps        int `json:"totalSteps"`
	RunningSteps      int `json:"runningSteps"`
	HealthySteps      int `json:"healthySteps"`
	FailedSteps       int `json:"failedSteps"`
	PendingSteps      int `json:"pendingSteps"`
}

type Overview struct {
	UpdatedAt  time.Time        `json:"updatedAt"`
	Summary    OverviewSummary  `json:"summary"`
	Executions []ExecutionState `json:"executions"`
}
