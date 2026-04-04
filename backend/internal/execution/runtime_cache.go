package execution

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/cachehub"
	"github.com/babelsuite/babelsuite/internal/logstream"
)

const activeExecutionsCacheKey = "execution-runtime:active"

type executionCache interface {
	Enabled() bool
	ReadJSON(ctx context.Context, key string, target any) (bool, error)
	WriteJSON(ctx context.Context, key string, value any, ttl time.Duration) error
}

func (s *Service) ConfigureRuntimeCache(hub *cachehub.Hub, ttl time.Duration) {
	s.mu.Lock()
	s.runtimeCache = hub
	if ttl > 0 {
		s.runtimeTTL = ttl
	}
	s.mu.Unlock()
	s.restoreActiveExecutions()
}

func (s *Service) restoreActiveExecutions() {
	s.mu.Lock()
	hub := s.runtimeCache
	s.mu.Unlock()
	if hub == nil || !hub.Enabled() {
		return
	}

	var persisted []PersistedExecution
	ok, err := hub.ReadJSON(context.Background(), activeExecutionsCacheKey, &persisted)
	if err != nil || !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range persisted {
		if item.Record.ID == "" {
			continue
		}
		if _, exists := s.executions[item.Record.ID]; exists {
			continue
		}
		state := &executionState{
			record:    item.Record,
			total:     item.Total,
			completed: item.Completed,
		}
		s.executions[item.Record.ID] = state
		s.order = append(s.order, item.Record.ID)
		s.logs.Open(item.Record.ID)
		for _, line := range item.Logs {
			s.logs.Append(item.Record.ID, line)
		}
	}
}

func (s *Service) persistExecutionCache() {
	s.mu.Lock()
	hub := s.runtimeCache
	ttl := s.runtimeTTL
	persisted := make([]PersistedExecution, 0, len(s.executions))
	for _, state := range s.executions {
		if state == nil {
			continue
		}
		if state.record.Status == "Healthy" || state.record.Status == "Failed" {
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

	if hub == nil || !hub.Enabled() {
		return
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_ = hub.WriteJSON(context.Background(), activeExecutionsCacheKey, persisted, ttl)
}

func (s *Service) executionLogLinesLocked(executionID string) []logstream.Line {
	lines, err := s.logs.Snapshot(executionID)
	if err != nil {
		return nil
	}
	return lines
}
