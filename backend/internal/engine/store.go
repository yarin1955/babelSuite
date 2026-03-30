package engine

import (
	"context"
	"sync"
	"time"
)

type Store struct {
	mu    sync.RWMutex
	state State
	subs  map[chan Overview]struct{}
}

func NewStore() *Store {
	return &Store{
		state: State{
			Executions: make(map[string]ExecutionState),
			Order:      []string{},
		},
		subs: make(map[chan Overview]struct{}),
	}
}

func (s *Store) Dispatch(action Action) {
	if action == nil {
		return
	}

	s.mu.Lock()
	action.apply(&s.state)
	s.state.UpdatedAt = time.Now().UTC()
	snapshot := buildOverview(s.state)
	subscribers := cloneSubscribers(s.subs)
	s.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- snapshot:
		default:
		}
	}
}

func (s *Store) Overview() Overview {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return buildOverview(s.state)
}

func (s *Store) Subscribe(ctx context.Context) <-chan Overview {
	ch := make(chan Overview, 8)

	s.mu.Lock()
	ch <- buildOverview(s.state)
	s.subs[ch] = struct{}{}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()

		delete(s.subs, ch)
		close(ch)
	}()

	return ch
}

func buildOverview(state State) Overview {
	overview := Overview{
		UpdatedAt:  state.UpdatedAt,
		Executions: make([]ExecutionState, 0, len(state.Order)),
	}

	for index := len(state.Order) - 1; index >= 0; index-- {
		execution, ok := state.Executions[state.Order[index]]
		if !ok {
			continue
		}

		overview.Executions = append(overview.Executions, cloneExecution(execution))
		overview.Summary.TotalExecutions++
		overview.Summary.TotalSteps += execution.TotalSteps
		overview.Summary.RunningSteps += execution.RunningSteps
		overview.Summary.HealthySteps += execution.HealthySteps
		overview.Summary.FailedSteps += execution.FailedSteps
		overview.Summary.PendingSteps += execution.PendingSteps

		switch execution.Status {
		case "Healthy":
			overview.Summary.HealthyExecutions++
		case "Failed":
			overview.Summary.FailedExecutions++
		default:
			overview.Summary.BootingExecutions++
		}
	}

	return overview
}

func cloneExecution(input ExecutionState) ExecutionState {
	output := input
	output.Steps = make([]StepState, len(input.Steps))
	for index, step := range input.Steps {
		output.Steps[index] = step
		output.Steps[index].DependsOn = append([]string{}, step.DependsOn...)
	}
	return output
}

func cloneSubscribers(input map[chan Overview]struct{}) []chan Overview {
	if len(input) == 0 {
		return nil
	}

	output := make([]chan Overview, 0, len(input))
	for subscriber := range input {
		output = append(output, subscriber)
	}
	return output
}
