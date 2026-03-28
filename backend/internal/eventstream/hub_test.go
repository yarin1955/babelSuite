package eventstream

import (
	"context"
	"testing"
	"time"
)

func TestHubReplaysAndStreams(t *testing.T) {
	t.Parallel()

	hub := NewHub[string]()
	hub.Open("sandboxes")
	hub.Append("sandboxes", "first")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := hub.Subscribe(ctx, "sandboxes", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case record := <-stream:
		if record.ID != 1 || record.Payload != "first" {
			t.Fatalf("unexpected replay record: %+v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay record")
	}

	hub.Append("sandboxes", "second")

	select {
	case record := <-stream:
		if record.ID != 2 || record.Payload != "second" {
			t.Fatalf("unexpected live record: %+v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live record")
	}
}
