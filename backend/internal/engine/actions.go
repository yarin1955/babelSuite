package engine

type Action interface {
	apply(state *State)
}

type UpsertExecutionAction struct {
	Execution ExecutionState
}

func (a UpsertExecutionAction) apply(state *State) {
	if state.Executions == nil {
		state.Executions = make(map[string]ExecutionState)
	}

	if _, exists := state.Executions[a.Execution.ID]; !exists {
		state.Order = append(state.Order, a.Execution.ID)
	}
	state.Executions[a.Execution.ID] = cloneExecution(a.Execution)
}
