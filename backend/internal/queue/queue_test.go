package queue

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryQueueHonorsDependencies(t *testing.T) {
	q := NewMemory(context.Background(), 1)
	defer q.Close()

	var mu sync.Mutex
	order := []string{}
	doneCh := make(chan struct{})

	err := q.Enqueue([]Task{
		{
			ID: "first",
			Run: func(context.Context) error {
				mu.Lock()
				order = append(order, "first")
				mu.Unlock()
				return nil
			},
		},
		{
			ID:           "second",
			Dependencies: []string{"first"},
			Run: func(context.Context) error {
				mu.Lock()
				order = append(order, "second")
				mu.Unlock()
				close(doneCh)
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dependent task")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("unexpected execution order: %#v", order)
	}
}
