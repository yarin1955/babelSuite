package agent

import (
	"path/filepath"
	"testing"
)

func TestRegistryPersistsRegistrationAndHeartbeat(t *testing.T) {
	store := NewFileRuntimeStore(filepath.Join(t.TempDir(), "agents.yaml"))
	registry := NewRegistry(store)

	record := registry.Register(RegisterRequest{
		AgentID:      "worker-1",
		Name:         "Worker 1",
		HostURL:      "http://127.0.0.1:8091",
		Capabilities: []string{"service", "test"},
	})
	if record.AgentID != "worker-1" {
		t.Fatalf("expected agent id to round-trip, got %q", record.AgentID)
	}

	heartbeat, ok := registry.Heartbeat("worker-1")
	if !ok {
		t.Fatal("expected heartbeat to find the persisted worker")
	}
	if heartbeat.LastHeartbeatAt.IsZero() {
		t.Fatal("expected heartbeat timestamp to be recorded")
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load runtime state: %v", err)
	}
	if len(state.Agents) == 0 {
		t.Fatal("expected registered worker to be persisted into platform settings")
	}

	var worker Registration
	found := false
	for _, agent := range state.Agents {
		if agent.AgentID == "worker-1" {
			worker = agent
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find persisted worker agent")
	}
	if worker.LastHeartbeatAt.IsZero() {
		t.Fatal("expected last heartbeat to be persisted")
	}
	if len(worker.Capabilities) != 2 {
		t.Fatalf("expected runtime capabilities to be persisted, got %v", worker.Capabilities)
	}
}
