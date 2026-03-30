package eventstream

import (
	"context"
	"errors"
	"sync"
)

var ErrNotFound = errors.New("event stream not found")

type Record[T any] struct {
	ID      int `json:"id"`
	Payload T   `json:"payload"`
}

type subscriber[T any] struct {
	ch chan Record[T]
}

type stream[T any] struct {
	list []T
	subs map[*subscriber[T]]struct{}
}

type Hub[T any] struct {
	mu      sync.Mutex
	streams map[string]*stream[T]
}

func NewHub[T any]() *Hub[T] {
	return &Hub[T]{
		streams: make(map[string]*stream[T]),
	}
}

func (h *Hub[T]) Open(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ensureStream(key)
}

func (h *Hub[T]) Append(key string, payload T) Record[T] {
	h.mu.Lock()
	itemStream := h.ensureStream(key)
	itemStream.list = append(itemStream.list, payload)
	record := Record[T]{
		ID:      len(itemStream.list),
		Payload: payload,
	}
	subscribers := cloneSubscribers(itemStream.subs)
	h.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber.ch <- record:
		default:
		}
	}

	return record
}

func (h *Hub[T]) Subscribe(ctx context.Context, key string, since int) (<-chan Record[T], error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	itemStream, ok := h.streams[key]
	if !ok {
		return nil, ErrNotFound
	}

	if since < 0 {
		since = 0
	}

	bufferSize := len(itemStream.list) - since + 16
	if bufferSize < 32 {
		bufferSize = 32
	}

	sub := &subscriber[T]{ch: make(chan Record[T], bufferSize)}
	for index, item := range itemStream.list {
		if index+1 <= since {
			continue
		}
		sub.ch <- Record[T]{
			ID:      index + 1,
			Payload: item,
		}
	}

	itemStream.subs[sub] = struct{}{}

	go func() {
		<-ctx.Done()
		h.mu.Lock()
		defer h.mu.Unlock()

		if existing := h.streams[key]; existing != nil {
			delete(existing.subs, sub)
		}
	}()

	return sub.ch, nil
}

func (h *Hub[T]) Len(key string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	itemStream, ok := h.streams[key]
	if !ok {
		return 0
	}
	return len(itemStream.list)
}

func (h *Hub[T]) ensureStream(key string) *stream[T] {
	itemStream, ok := h.streams[key]
	if ok {
		return itemStream
	}

	itemStream = &stream[T]{
		list: []T{},
		subs: make(map[*subscriber[T]]struct{}),
	}
	h.streams[key] = itemStream
	return itemStream
}

func cloneSubscribers[T any](input map[*subscriber[T]]struct{}) []*subscriber[T] {
	if len(input) == 0 {
		return nil
	}

	output := make([]*subscriber[T], 0, len(input))
	for subscriber := range input {
		output = append(output, subscriber)
	}
	return output
}
