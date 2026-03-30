package watchers

import (
	"github.com/babelsuite/babelsuite/internal/engine"
	"github.com/babelsuite/babelsuite/internal/execution"
)

type ExecutionWatcher struct {
	store *engine.Store
}

func NewExecutionWatcher(store *engine.Store) *ExecutionWatcher {
	return &ExecutionWatcher{store: store}
}

func (w *ExecutionWatcher) SyncExecution(snapshot execution.Snapshot) {
	if w == nil || w.store == nil {
		return
	}

	steps := make([]engine.StepState, len(snapshot.Steps))
	for index, step := range snapshot.Steps {
		steps[index] = engine.StepState{
			ID:        step.ID,
			Name:      step.Name,
			Kind:      step.Kind,
			Status:    step.Status,
			DependsOn: append([]string{}, step.DependsOn...),
		}
	}

	w.store.Dispatch(engine.UpsertExecutionAction{
		Execution: engine.ExecutionState{
			ID:            snapshot.ID,
			SuiteID:       snapshot.SuiteID,
			SuiteTitle:    snapshot.SuiteTitle,
			Profile:       snapshot.Profile,
			Trigger:       snapshot.Trigger,
			Status:        snapshot.Status,
			Duration:      snapshot.Duration,
			StartedAt:     snapshot.StartedAt,
			UpdatedAt:     snapshot.UpdatedAt,
			TotalSteps:    snapshot.TotalSteps,
			RunningSteps:  snapshot.RunningSteps,
			HealthySteps:  snapshot.HealthySteps,
			FailedSteps:   snapshot.FailedSteps,
			PendingSteps:  snapshot.PendingSteps,
			ProgressRatio: snapshot.ProgressRatio,
			Steps:         steps,
		},
	})
}
