package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/suites"
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
			Kind: "service",
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
			Kind: "test",
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
			Kind: "task",
		},
	}, func(logstream.Line) {}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if time.Since(start) < 400*time.Millisecond {
		t.Fatal("expected task step to simulate a real runner delay")
	}
}

func TestLocalRunnerEmitsTrafficSpecificLogs(t *testing.T) {
	r := NewLocal()
	lines := make([]logstream.Line, 0)

	err := r.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "storefront-browser-lab",
		SuiteTitle:  "Storefront Browser Lab",
		Profile:     "traffic.yaml",
		Node: StepNode{
			ID:   "promo-burst",
			Name: "promo-burst",
			Kind: "traffic",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(lines) < 5 {
		t.Fatalf("expected richer traffic logs, got %d lines", len(lines))
	}
	if lines[1].Text == "" {
		t.Fatal("expected non-empty traffic boot message")
	}
	foundThresholdLog := false
	for _, line := range lines {
		if line.Text != "" && (contains(line.Text, "threshold") || contains(line.Text, "ramp")) {
			foundThresholdLog = true
			break
		}
	}
	if !foundThresholdLog {
		t.Fatal("expected traffic logs to mention thresholds or ramp behavior")
	}
}

func TestLocalRunnerExecutesNativeTrafficPlan(t *testing.T) {
	runner := NewLocal()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	lines := make([]logstream.Line, 0)
	err := runner.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "local.yaml",
		Load: &suites.LoadSpec{
			Variant:  "traffic.baseline",
			PlanPath: "traffic/smoke.star",
			Target:   server.URL,
			Users: []suites.LoadUser{
				{
					Name:   "probe",
					Weight: 1,
					Wait:   suites.LoadWait{Mode: "constant", Seconds: 0},
					Tasks: []suites.LoadTask{
						{
							Name:   "health",
							Weight: 1,
							Request: suites.LoadRequest{
								Method: "GET",
								Path:   "/health",
								Name:   "health",
							},
							Checks: []suites.LoadThreshold{
								{Metric: "status", Op: "==", Value: 200},
								{Metric: "latency.p95_ms", Op: "<", Value: 1000, Sampler: "health"},
							},
						},
					},
				},
			},
			Stages: []suites.LoadStage{
				{Duration: 250 * time.Millisecond, Users: 2, SpawnRate: 2, Stop: true},
			},
			Thresholds: []suites.LoadThreshold{
				{Metric: "http.error_rate", Op: "<=", Value: 0},
			},
		},
		Node: StepNode{
			ID:      "checkout-baseline",
			Name:    "checkout-baseline",
			Kind:    "traffic",
			Variant: "traffic.baseline",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if requests.Load() == 0 {
		t.Fatal("expected native traffic executor to hit the target server")
	}
	foundNativePlanLog := false
	foundExtendedLatency := false
	foundThroughput := false
	foundHistogram := false
	foundStageSummary := false
	for _, line := range lines {
		if contains(line.Text, "loaded native traffic plan") || contains(line.Text, "native traffic run completed") {
			foundNativePlanLog = true
		}
		if contains(line.Text, "p50=") && contains(line.Text, "p99=") && contains(line.Text, "avg=") {
			foundExtendedLatency = true
		}
		if contains(line.Text, "throughput avg=") {
			foundThroughput = true
		}
		if contains(line.Text, "latency histogram") {
			foundHistogram = true
		}
		if contains(line.Text, "stage 1 summary") {
			foundStageSummary = true
		}
	}
	if !foundNativePlanLog {
		t.Fatal("expected native traffic execution logs")
	}
	if !foundExtendedLatency {
		t.Fatal("expected extended latency metrics in traffic logs")
	}
	if !foundThroughput {
		t.Fatal("expected throughput summary in traffic logs")
	}
	if !foundHistogram {
		t.Fatal("expected latency histogram in traffic logs")
	}
	if !foundStageSummary {
		t.Fatal("expected per-stage traffic summary in traffic logs")
	}
}

func TestLocalRunnerExecutesSyntheticTrafficForSuiteLocalTargets(t *testing.T) {
	runner := NewLocal()
	lines := make([]logstream.Line, 0)

	err := runner.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "composite-readiness",
		SuiteTitle:  "Composite Readiness",
		Profile:     "local.yaml",
		Load: &suites.LoadSpec{
			Variant:  "traffic.baseline",
			PlanPath: "traffic/checkout_baseline.star",
			Target:   "http://payment_gateway:8080",
			Users: []suites.LoadUser{
				{
					Name:   "shopper",
					Weight: 1,
					Wait:   suites.LoadWait{Mode: "constant", Seconds: 0},
					Tasks: []suites.LoadTask{
						{
							Name:   "create-charge",
							Weight: 1,
							Request: suites.LoadRequest{
								Method: "POST",
								Path:   "/charges",
								Name:   "create-charge",
							},
							Checks: []suites.LoadThreshold{
								{Metric: "status", Op: "==", Value: 200},
								{Metric: "latency.p95_ms", Op: "<", Value: 600, Sampler: "create-charge"},
							},
						},
					},
				},
			},
			Stages: []suites.LoadStage{
				{Duration: 150 * time.Millisecond, Users: 2, SpawnRate: 2, Stop: true},
			},
			Thresholds: []suites.LoadThreshold{
				{Metric: "http.error_rate", Op: "<", Value: 0.01},
			},
		},
		Node: StepNode{
			ID:      "payments/checkout_baseline",
			Name:    "payments/checkout_baseline",
			Kind:    "traffic",
			Variant: "traffic.baseline",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	foundSyntheticLog := false
	for _, line := range lines {
		if contains(line.Text, "suite-local symbolic service") {
			foundSyntheticLog = true
			break
		}
	}
	if !foundSyntheticLog {
		t.Fatal("expected synthetic internal-target traffic log")
	}
}

func TestLocalRunnerPassesStepEvaluationControls(t *testing.T) {
	runner := NewLocal()
	expectExit := 0
	lines := make([]logstream.Line, 0)

	err := runner.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "local.yaml",
		Evaluation: &suites.StepEvaluation{
			ExpectExit: &expectExit,
			ExpectLogs: []string{"successfully"},
			FailOnLogs: []string{"FATAL ERROR"},
		},
		Node: StepNode{
			ID:      "seed-data",
			Name:    "seed-data",
			Kind:    "task",
			Variant: "task.run",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	foundEvaluationLog := false
	for _, line := range lines {
		if contains(line.Text, "evaluation controls passed") {
			foundEvaluationLog = true
			break
		}
	}
	if !foundEvaluationLog {
		t.Fatal("expected evaluation success log")
	}
}

func TestLocalRunnerFailsWhenExpectedLogsAreMissing(t *testing.T) {
	runner := NewLocal()
	lines := make([]logstream.Line, 0)

	err := runner.Run(context.Background(), StepSpec{
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "local.yaml",
		Evaluation: &suites.StepEvaluation{
			ExpectLogs: []string{"THIS STRING DOES NOT EXIST"},
		},
		Node: StepNode{
			ID:      "smoke",
			Name:    "smoke",
			Kind:    "test",
			Variant: "test.run",
		},
	}, func(line logstream.Line) {
		lines = append(lines, line)
	})
	if err == nil {
		t.Fatal("expected evaluation failure")
	}
	foundFailureLog := false
	for _, line := range lines {
		if contains(line.Text, "expected logs containing") {
			foundFailureLog = true
			break
		}
	}
	if !foundFailureLog {
		t.Fatal("expected evaluation failure log")
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
