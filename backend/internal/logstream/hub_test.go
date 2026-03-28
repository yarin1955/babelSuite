package logstream

import (
	"context"
	"testing"
	"time"
)

func TestHubReplaysAndStreams(t *testing.T) {
	hub := NewHub()
	hub.Append("run-1", Line{Source: "db", Timestamp: "00:00", Level: "info", Text: "booting"})

	stream, err := hub.Subscribe(context.Background(), "run-1", 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case record := <-stream:
		if record.ID != 1 {
			t.Fatalf("expected replay id 1, got %d", record.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay")
	}

	hub.Append("run-1", Line{Source: "db", Timestamp: "00:01", Level: "info", Text: "healthy"})

	select {
	case record := <-stream:
		if record.Line.Text != "healthy" {
			t.Fatalf("expected live log line, got %q", record.Line.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live line")
	}
}
