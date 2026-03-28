package engine

import (
	"testing"
	"time"
)

func TestStoreBuildsOverviewCounts(t *testing.T) {
	store := NewStore()
	store.Dispatch(UpsertExecutionAction{
		Execution: ExecutionState{
			ID:           "run-1",
			SuiteID:      "payment-suite",
			SuiteTitle:   "Payment Suite",
			Status:       "Booting",
			Duration:     "00:12",
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			TotalSteps:   3,
			RunningSteps: 1,
			HealthySteps: 1,
			PendingSteps: 1,
		},
	})

	overview := store.Overview()
	if overview.Summary.TotalExecutions != 1 {
		t.Fatalf("expected 1 execution, got %d", overview.Summary.TotalExecutions)
	}
	if overview.Summary.BootingExecutions != 1 {
		t.Fatalf("expected 1 booting execution, got %d", overview.Summary.BootingExecutions)
	}
	if overview.Summary.RunningSteps != 1 {
		t.Fatalf("expected 1 running step, got %d", overview.Summary.RunningSteps)
	}
}
