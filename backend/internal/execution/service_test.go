package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type captureObserver struct {
	events chan Snapshot
}

func (o *captureObserver) SyncExecution(snapshot Snapshot) {
	select {
	case o.events <- snapshot:
	default:
	}
}

type staticSuiteSource struct {
	items map[string]suites.Definition
}

func (s staticSuiteSource) List() []suites.Definition {
	result := make([]suites.Definition, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

func (s staticSuiteSource) Get(id string) (*suites.Definition, error) {
	item, ok := s.items[id]
	if !ok {
		return nil, suites.ErrNotFound
	}
	return &item, nil
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

func TestCreateExecutionRejectsInvalidTopology(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"broken-suite": {
				ID:        "broken-suite",
				Title:     "Broken Suite",
				SuiteStar: "api = container.run(name=\"api\", after=[\"db\"])\n",
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "broken-suite",
		Profile: "local.yaml",
	})
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected ErrInvalidTopology, got %v", err)
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
