package execution

import (
	"context"
	"sort"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type RuntimeStore interface {
	LoadExecutionRuntime(ctx context.Context) ([]PersistedExecution, error)
	SaveExecutionRuntime(ctx context.Context, executions []PersistedExecution) error
}

type PersistedExecution struct {
	Record    ExecutionRecord  `json:"record"`
	Total     int              `json:"total"`
	Completed int              `json:"completed"`
	Logs      []logstream.Line `json:"logs"`
}

func (s *Service) ConfigureRuntimeStore(store RuntimeStore) {
	s.mu.Lock()
	s.runtimeStore = store
	s.mu.Unlock()
	s.restorePersistedExecutions()
}

func (s *Service) restorePersistedExecutions() {
	s.mu.Lock()
	store := s.runtimeStore
	s.mu.Unlock()
	if store == nil {
		return
	}

	persisted, err := store.LoadExecutionRuntime(context.Background())
	if err != nil || len(persisted) == 0 {
		return
	}

	sort.Slice(persisted, func(i, j int) bool {
		return persisted[i].Record.StartedAt.Before(persisted[j].Record.StartedAt)
	})

	s.mu.Lock()
	s.executions = make(map[string]*executionState, len(persisted))
	s.order = s.order[:0]
	for _, item := range persisted {
		if item.Record.ID == "" {
			continue
		}
		statuses, completed := buildStepStatus(item.Record.Suite.Topology, item.Record.Events)
		state := &executionState{
			record:     item.Record,
			total:      item.Total,
			completed:  completed,
			stepStatus: statuses,
		}
		if state.total == 0 {
			state.total = len(item.Record.Suite.Topology)
		}
		if state.completed == 0 && item.Completed > 0 {
			state.completed = item.Completed
		}
		s.executions[item.Record.ID] = state
		s.order = append(s.order, item.Record.ID)
		s.logs.Open(item.Record.ID)
		for _, line := range item.Logs {
			s.logs.Append(item.Record.ID, line)
		}
	}
	s.mu.Unlock()
}

func (s *Service) persistExecutionRuntime() {
	s.persistExecutionStore()
	s.persistExecutionCache()
}

func (s *Service) persistExecutionStore() {
	s.mu.Lock()
	store := s.runtimeStore
	persisted := make([]PersistedExecution, 0, len(s.executions))
	for _, state := range s.executions {
		if state == nil {
			continue
		}
		persisted = append(persisted, PersistedExecution{
			Record:    state.record,
			Total:     state.total,
			Completed: state.completed,
			Logs:      s.executionLogLinesLocked(state.record.ID),
		})
	}
	s.mu.Unlock()

	if store == nil {
		return
	}

	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = store.SaveExecutionRuntime(saveCtx, persisted)
}
