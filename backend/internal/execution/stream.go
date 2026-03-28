package execution

import (
	"context"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type StreamEvent struct {
	ID              int            `json:"id"`
	ExecutionID     string         `json:"executionId"`
	ExecutionStatus string         `json:"executionStatus"`
	Duration        string         `json:"duration"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	Event           ExecutionEvent `json:"event"`
}

func (s *Service) SubscribeEvents(ctx context.Context, executionID string, since int) (<-chan StreamEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.executions[executionID]
	if item == nil {
		return nil, ErrExecutionNotFound
	}

	if since < 0 {
		since = 0
	}

	bufferSize := len(item.record.Events) - since + 16
	if bufferSize < 32 {
		bufferSize = 32
	}

	ch := make(chan StreamEvent, bufferSize)
	for index, event := range item.record.Events {
		if index+1 <= since {
			continue
		}

		ch <- StreamEvent{
			ID:              index + 1,
			ExecutionID:     executionID,
			ExecutionStatus: item.record.Status,
			Duration:        s.durationLocked(item),
			UpdatedAt:       item.record.UpdatedAt,
			Event:           event,
		}
	}

	if s.subs[executionID] == nil {
		s.subs[executionID] = make(map[chan StreamEvent]struct{})
	}
	s.subs[executionID][ch] = struct{}{}

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()

		if subscribers := s.subs[executionID]; subscribers != nil {
			delete(subscribers, ch)
			if len(subscribers) == 0 {
				delete(s.subs, executionID)
			}
		}
	}()

	return ch, nil
}

func (s *Service) SubscribeLogs(ctx context.Context, executionID string, since int) (<-chan logstream.Record, error) {
	s.mu.Lock()
	item := s.executions[executionID]
	s.mu.Unlock()
	if item == nil {
		return nil, ErrExecutionNotFound
	}

	return s.logs.Subscribe(ctx, executionID, since)
}

func (s *Service) publish(event StreamEvent, subscribers []chan StreamEvent) {
	for _, subscriber := range subscribers {
		select {
		case subscriber <- event:
		default:
			// Fall back to snapshot refresh if a client stops consuming.
		}
	}
}

func collectSubscribers(subscribers map[chan StreamEvent]struct{}) []chan StreamEvent {
	if len(subscribers) == 0 {
		return nil
	}

	result := make([]chan StreamEvent, 0, len(subscribers))
	for subscriber := range subscribers {
		result = append(result, subscriber)
	}
	return result
}
