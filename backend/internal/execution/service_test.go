package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type captureObserver struct {
	events chan Snapshot
}

type stubPlatformSource struct {
	settings *platform.PlatformSettings
	err      error
}

type stubRuntimeStore struct {
	executions []PersistedExecution
}

func (s stubPlatformSource) Load() (*platform.PlatformSettings, error) {
	return s.settings, s.err
}

func (s *stubRuntimeStore) LoadExecutionRuntime(_ context.Context) ([]PersistedExecution, error) {
	return append([]PersistedExecution{}, s.executions...), nil
}

func (s *stubRuntimeStore) SaveExecutionRuntime(_ context.Context, executions []PersistedExecution) error {
	s.executions = append([]PersistedExecution{}, executions...)
	return nil
}

func (o *captureObserver) SyncExecution(snapshot Snapshot) {
	select {
	case o.events <- snapshot:
	default:
	}
}

func TestCreateExecutionUsesExplicitRemoteBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID:     "remote-agent",
					Name:        "Remote Worker",
					Type:        "remote-agent",
					Description: "Dispatches steps to a worker process.",
					Enabled:     true,
					Default:     true,
					HostURL:     "http://worker.example",
				},
			},
		},
	})
	defer service.Close()

	registry := agent.NewRegistry(nil)
	coordinator := agent.NewCoordinator(registry, service)
	service.ConfigureRemoteWorkers(registry, coordinator)

	controlPlane := httptest.NewServer(agent.NewGateway(registry, coordinator))
	defer controlPlane.Close()

	client := agent.NewControlPlaneClient(controlPlane.URL, controlPlane.Client())
	if err := client.Register(context.Background(), agent.RegisterRequest{
		AgentID: "remote-agent",
		Name:    "Remote Worker",
		HostURL: "http://worker.example",
	}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	workerService := agent.NewService(agent.Info{
		AgentID: "remote-agent",
		Name:    "Remote Worker",
		Status:  "ready",
	}, agent.ExecutorFunc(func(ctx context.Context, request agent.StepRequest, emit func(line logstream.Line)) error {
		emit(logstream.Line{Source: request.Node.ID, Level: "info", Text: "remote worker running"})
		return nil
	}))
	worker := agent.NewWorker("remote-agent", 20*time.Millisecond, client, workerService)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go func() { _ = worker.Run(workerCtx) }()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
		Backend: "remote-agent",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.BackendID != "remote-agent" {
		t.Fatalf("expected remote-agent backend id, got %q", execution.BackendID)
	}
	if execution.Backend != "Remote Worker" {
		t.Fatalf("expected backend label to round-trip, got %q", execution.Backend)
	}
}

func TestConfigureRuntimeStoreRestoresPersistedExecutions(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	store := &stubRuntimeStore{
		executions: []PersistedExecution{
			{
				Record: ExecutionRecord{
					ID:        "run-persisted",
					Suite:     ExecutionSuite{ID: "payment-suite", Title: "Payment Suite"},
					Profile:   "local.yaml",
					Backend:   "Remote Worker",
					Status:    "Booting",
					Trigger:   "Manual",
					StartedAt: time.Now().UTC().Add(-time.Minute),
					UpdatedAt: time.Now().UTC(),
				},
				Total:     3,
				Completed: 1,
				Logs: []logstream.Line{
					{Source: "db", Level: "info", Text: "restored"},
				},
			},
		},
	}

	service.ConfigureRuntimeStore(store)

	executions := service.ListExecutions()
	if len(executions) != 1 {
		t.Fatalf("expected one restored execution, got %d", len(executions))
	}
	if executions[0].ID != "run-persisted" {
		t.Fatalf("expected restored execution id, got %q", executions[0].ID)
	}

	record, err := service.GetExecution("run-persisted")
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if record.Backend != "Remote Worker" {
		t.Fatalf("expected restored backend, got %q", record.Backend)
	}
}

func TestCreateExecutionRejectsUnknownBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "local-docker",
					Name:    "Local Docker",
					Type:    "local",
					Enabled: true,
					Default: true,
				},
			},
		},
	})
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
		Backend: "missing-backend",
	})
	if !errors.Is(err, ErrBackendNotFound) {
		t.Fatalf("expected ErrBackendNotFound, got %v", err)
	}
}

func TestCreateExecutionRejectsUnavailableBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "remote-agent",
					Name:    "Remote Worker",
					Type:    "remote-agent",
					Enabled: true,
					Default: true,
					HostURL: "http://127.0.0.1:1",
				},
			},
		},
	})
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
	})
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("expected ErrBackendUnavailable, got %v", err)
	}
}

func TestCreateExecutionRejectsProfileFromAnotherSuite(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "perf.yaml",
	})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestCreateExecutionUsesSuiteSpecificDefaultProfile(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.Profile != "local.yaml" {
		t.Fatalf("expected suite default profile, got %q", execution.Profile)
	}
}

func TestCreateExecutionUsesPersistedSuiteProfileSet(t *testing.T) {
	profileService := profiles.NewService(suites.NewService(), profiles.NewMemoryStore())
	if _, err := profileService.CreateProfile("payment-suite", profiles.UpsertRequest{
		Name:        "Holiday Freeze",
		FileName:    "holiday.yaml",
		Description: "Freeze routing for end-of-quarter reconciliation.",
		Scope:       "Staging",
		YAML:        "env:\n  LEDGER_PERIOD: holiday\nservices:\n  workerReplicaCount: 2\n",
		ExtendsID:   "base",
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	service := NewService(profileService)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "holiday.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.Profile != "holiday.yaml" {
		t.Fatalf("expected holiday.yaml profile, got %q", execution.Profile)
	}
}

func TestCreateExecutionSyncsObservers(t *testing.T) {
	observer := &captureObserver{events: make(chan Snapshot, 8)}
	service := NewService(suites.NewService(), observer)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case snapshot := <-observer.events:
			if snapshot.ID != execution.ID {
				continue
			}
			if snapshot.Profile != "year.yaml" {
				t.Fatalf("expected year.yaml profile, got %q", snapshot.Profile)
			}
			if snapshot.TotalSteps == 0 {
				t.Fatal("expected execution snapshot to include topology steps")
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for observer sync")
		}
	}
}

func TestSubscribeEventsReplaysAndStreams(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	stream, err := service.SubscribeEvents(context.Background(), "run-1043", 0)
	if err != nil {
		t.Fatalf("subscribe existing execution: %v", err)
	}

	select {
	case event := <-stream:
		if event.ID != 1 {
			t.Fatalf("expected first replay event id 1, got %d", event.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay event")
	}

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	liveStream, err := service.SubscribeEvents(context.Background(), execution.ID, 0)
	if err != nil {
		t.Fatalf("subscribe live execution: %v", err)
	}

	select {
	case event := <-liveStream:
		if event.Event.Source == "" {
			t.Fatal("expected streamed event source")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live event")
	}
}

func TestSubscribeLogsReplaysAndStreams(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	stream, err := service.SubscribeLogs(context.Background(), "run-1043", 0)
	if err != nil {
		t.Fatalf("subscribe existing execution logs: %v", err)
	}

	select {
	case line := <-stream:
		if line.ID != 1 {
			t.Fatalf("expected first replay log id 1, got %d", line.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay log line")
	}

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	liveStream, err := service.SubscribeLogs(context.Background(), execution.ID, 0)
	if err != nil {
		t.Fatalf("subscribe live execution logs: %v", err)
	}

	select {
	case line := <-liveStream:
		if line.Line.Source == "" {
			t.Fatal("expected streamed log line source")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live log line")
	}
}

func TestStorefrontExecutionUsesScenarioOnlyTopology(t *testing.T) {
	observer := &captureObserver{events: make(chan Snapshot, 16)}
	service := NewService(suites.NewService(), observer)
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "storefront-browser-lab",
		Profile: "promo.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case snapshot := <-observer.events:
			foundScenario := false
			for _, step := range snapshot.Steps {
				if step.Kind == "load" {
					t.Fatal("did not expect storefront execution to include a load step")
				}
				if step.Kind == "scenario" && step.ID == "playwright-checkout" {
					foundScenario = true
				}
			}
			if foundScenario {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for storefront scenario step to appear in execution snapshot")
		}
	}
}

func TestGetExecutionIncludesSuiteSourceFiles(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "returns-control-plane",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(record.Suite.Folders) == 0 {
		t.Fatal("expected execution suite folders")
	}
	if len(record.Suite.SourceFiles) == 0 {
		t.Fatal("expected execution suite source files")
	}
	if len(record.Suite.APISurfaces) == 0 {
		t.Fatal("expected execution suite api surfaces")
	}

	foundMockFile := false
	for _, file := range record.Suite.SourceFiles {
		if file.Path != "mock/returns/create-return.cue" {
			continue
		}
		foundMockFile = true
		if !strings.Contains(file.Content, `@gen(`) {
			t.Fatalf("expected declarative mock generation content, got %q", file.Content)
		}
	}

	if !foundMockFile {
		t.Fatal("expected returns execution to include mock source file")
	}
}

func TestGetExecutionRendersGeneratedMockPreviewData(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "returns-control-plane",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}

	var preview string
	for _, surface := range record.Suite.APISurfaces {
		if surface.ID != "returns-api" {
			continue
		}
		for _, operation := range surface.Operations {
			if operation.ID != "create-return" {
				continue
			}
			for _, exchange := range operation.Exchanges {
				if exchange.Name == "approved-standard" {
					preview = exchange.ResponseBody
				}
			}
		}
	}

	if strings.TrimSpace(preview) == "" {
		t.Fatal("expected create-return preview body")
	}
	if strings.Contains(preview, "{{") {
		t.Fatalf("expected rendered preview body, got %q", preview)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(preview), &payload); err != nil {
		t.Fatalf("preview body should be valid json: %v", err)
	}

	returnID, _ := payload["returnId"].(string)
	traceID, _ := payload["traceId"].(string)
	servedAt, _ := payload["servedAt"].(string)
	returnID = strings.TrimSpace(returnID)
	traceID = strings.TrimSpace(traceID)
	servedAt = strings.TrimSpace(servedAt)
	if !strings.HasPrefix(returnID, "ret_") {
		t.Fatalf("expected generated returnId, got %q", returnID)
	}
	if traceID == "" {
		t.Fatal("expected generated traceId")
	}
	if servedAt == "" {
		t.Fatal("expected generated servedAt")
	}
}
