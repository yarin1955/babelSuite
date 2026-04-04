package logstream

import (
	"context"
	"errors"
	"sync"
)

var ErrNotFound = errors.New("log stream not found")

type subscriber struct {
	ch chan Record
}

type stream struct {
	list []Line
	subs map[*subscriber]struct{}
}

type Hub struct {
	mu      sync.Mutex
	streams map[string]*stream
}

func NewHub() *Hub {
	return &Hub{
		streams: make(map[string]*stream),
	}
}

func (h *Hub) Open(executionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ensureStream(executionID)
}

func (h *Hub) Append(executionID string, line Line) {
	h.mu.Lock()
	s := h.ensureStream(executionID)
	s.list = append(s.list, line)
	record := Record{
		ID:          len(s.list),
		ExecutionID: executionID,
		Line:        line,
	}
	subscribers := cloneSubscribers(s.subs)
	h.mu.Unlock()

	for _, sub := range subscribers {
		select {
		case sub.ch <- record:
		default:
		}
	}
}

func (h *Hub) Subscribe(ctx context.Context, executionID string, since int) (<-chan Record, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.streams[executionID]
	if !ok {
		return nil, ErrNotFound
	}

	if since < 0 {
		since = 0
	}

	bufferSize := len(s.list) - since + 16
	if bufferSize < 32 {
		bufferSize = 32
	}

	sub := &subscriber{ch: make(chan Record, bufferSize)}
	for index, line := range s.list {
		if index+1 <= since {
			continue
		}

		sub.ch <- Record{
			ID:          index + 1,
			ExecutionID: executionID,
			Line:        line,
		}
	}

	s.subs[sub] = struct{}{}

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		defer h.mu.Unlock()

		if existing := h.streams[executionID]; existing != nil {
			delete(existing.subs, sub)
		}
	}()

	return sub.ch, nil
}

func (h *Hub) Snapshot(executionID string) ([]Line, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s, ok := h.streams[executionID]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]Line{}, s.list...), nil
}

func (h *Hub) ensureStream(executionID string) *stream {
	s, ok := h.streams[executionID]
	if ok {
		return s
	}

	s = &stream{
		list: []Line{},
		subs: make(map[*subscriber]struct{}),
	}
	h.streams[executionID] = s
	return s
}

func cloneSubscribers(input map[*subscriber]struct{}) []*subscriber {
	if len(input) == 0 {
		return nil
	}

	output := make([]*subscriber, 0, len(input))
	for sub := range input {
		output = append(output, sub)
	}
	return output
}
