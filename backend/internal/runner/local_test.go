package runner

import (
	"context"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

func TestLocalRunnerEmitsLogs(t *testing.T) {
	r := NewLocal()
	lines := make([]logstream.Line, 0)

	err := r.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "year.yaml",
		Node: StepNode{
			ID:   "db",
			Name: "db",
			Kind: "container",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(lines) < 3 {
		t.Fatalf("expected runner logs, got %d lines", len(lines))
	}
}

func TestLocalRunnerHonorsCancellation(t *testing.T) {
	r := NewLocal()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Run(ctx, StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "year.yaml",
		Node: StepNode{
			ID:   "checkout-smoke",
			Name: "checkout-smoke",
			Kind: "scenario",
		},
	}, func(logstream.Line) {})
	if err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestLocalRunnerCompletesWithinExpectedWindow(t *testing.T) {
	r := NewLocal()
	start := time.Now()

	if err := r.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "year.yaml",
		Node: StepNode{
			ID:   "bootstrap-topics",
			Name: "bootstrap-topics",
			Kind: "script",
		},
	}, func(logstream.Line) {}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if time.Since(start) < 400*time.Millisecond {
		t.Fatal("expected scripted step to simulate a real runner delay")
	}
}
