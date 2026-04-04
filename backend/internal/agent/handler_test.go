package agent

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/logstream"
)

type captureObserver struct {
	states []string
	lines  []string
}

type stubAssignmentStore struct {
	snapshots []AssignmentSnapshot
}

func (s *stubAssignmentStore) LoadAssignmentRuntime(_ context.Context) ([]AssignmentSnapshot, error) {
	return append([]AssignmentSnapshot{}, s.snapshots...), nil
}

func (s *stubAssignmentStore) SaveAssignmentRuntime(_ context.Context, snapshots []AssignmentSnapshot) error {
	s.snapshots = append([]AssignmentSnapshot{}, snapshots...)
	return nil
}

func (o *captureObserver) AssignmentState(_ StepRequest, state string, _ string) {
	o.states = append(o.states, state)
}

func (o *captureObserver) AssignmentLog(_ StepRequest, line logstream.Line) {
	o.lines = append(o.lines, line.Text)
}

func TestGatewayRegistersAndHeartbeatsWorkers(t *testing.T) {
	registry := NewRegistry(nil)
	coordinator := NewCoordinator(registry, nil)
	server := httptest.NewServer(NewGateway(registry, coordinator))
	defer server.Close()

	client := NewControlPlaneClient(server.URL, server.Client())
	if err := client.Register(context.Background(), RegisterRequest{
		AgentID:      "worker-1",
		Name:         "Worker",
		HostURL:      "http://127.0.0.1:8091",
		Capabilities: []string{"container"},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := client.Heartbeat(context.Background(), "worker-1"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	response, err := server.Client().Get(server.URL + "/api/v1/agents")
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	defer response.Body.Close()

	var payload struct {
		Agents []Registration `json:"agents"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode registry list: %v", err)
	}
	if len(payload.Agents) != 1 {
		t.Fatalf("expected one registered worker, got %d", len(payload.Agents))
	}
	if payload.Agents[0].AgentID != "worker-1" {
		t.Fatalf("expected registered worker id, got %q", payload.Agents[0].AgentID)
	}
}

func TestControlPlaneClientClaimsReportsAndCompletesAssignments(t *testing.T) {
	registry := NewRegistry(nil)
	observer := &captureObserver{}
	coordinator := NewCoordinator(registry, observer)
	server := httptest.NewServer(NewGateway(registry, coordinator))
	defer server.Close()

	client := NewControlPlaneClient(server.URL, server.Client())
	if err := client.Register(context.Background(), RegisterRequest{
		AgentID: "worker-1",
		Name:    "Worker",
		HostURL: server.URL,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	submitted, err := coordinator.Submit(StepRequest{
		JobID:       "run-1:db",
		ExecutionID: "run-1",
		SuiteID:     "payment-suite",
		SuiteTitle:  "Payment Suite",
		Profile:     "local.yaml",
		BackendID:   "worker-1",
		LeaseTTL:    2 * time.Second,
		Node: StepNode{
			ID:   "db",
			Name: "db",
			Kind: "container",
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	claim, err := client.ClaimNext(context.Background(), "worker-1")
	if err != nil {
		t.Fatalf("claim next: %v", err)
	}
	if claim == nil || claim.JobID != submitted.JobID {
		t.Fatalf("expected claim %q, got %#v", submitted.JobID, claim)
	}

	if err := client.ReportState(context.Background(), submitted.JobID, StateReport{
		AgentID: "worker-1",
		State:   "running",
	}); err != nil {
		t.Fatalf("report state: %v", err)
	}
	if err := client.ReportLog(context.Background(), submitted.JobID, "worker-1", logstream.Line{
		Source: "db",
		Level:  "info",
		Text:   "boot",
	}); err != nil {
		t.Fatalf("report log: %v", err)
	}
	if err := client.Complete(context.Background(), submitted.JobID, CompleteRequest{
		AgentID: "worker-1",
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if err := coordinator.Wait(context.Background(), submitted.JobID); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if len(observer.states) == 0 || observer.states[0] != "running" {
		t.Fatalf("expected running state report, got %v", observer.states)
	}
	if len(observer.lines) == 0 || observer.lines[0] != "boot" {
		t.Fatalf("expected boot log report, got %v", observer.lines)
	}
}

func TestCoordinatorConfigureStoreRestoresAssignments(t *testing.T) {
	registry := NewRegistry(nil)
	coordinator := NewCoordinator(registry, nil)
	store := &stubAssignmentStore{
		snapshots: []AssignmentSnapshot{
			{
				Request: StepRequest{
					JobID:       "run-2:db",
					ExecutionID: "run-2",
					SuiteID:     "payment-suite",
					SuiteTitle:  "Payment Suite",
					Profile:     "local.yaml",
					BackendID:   "worker-1",
					Node: StepNode{
						ID:   "db",
						Name: "db",
						Kind: "container",
					},
				},
				Status: AssignmentPending,
			},
		},
	}

	coordinator.ConfigureStore(store)

	claim, ok := coordinator.Claim("worker-1")
	if !ok {
		t.Fatal("expected restored assignment to be claimable")
	}
	if claim.JobID != "run-2:db" {
		t.Fatalf("expected restored job id, got %q", claim.JobID)
	}
}
