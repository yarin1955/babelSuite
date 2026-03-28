package execution

import (
	"context"
	"errors"
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
