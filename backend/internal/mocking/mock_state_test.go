package mocking

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func newTestService(suite suites.Definition) *Service {
	return &Service{
		suites:     staticSuiteReader{suite: suite},
		state:      make(map[string]map[string]string),
		suiteState: make(map[string]map[string]struct{}),
	}
}

func TestLoadState_NilConfig(t *testing.T) {
	// C1: nil config → empty non-nil map
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	got := s.loadState("any-key", nil)
	if got == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLoadState_DefaultsKeyNotStored(t *testing.T) {
	// C2: defaults set, key not stored → returns clone of defaults
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	config := &suites.MockState{Defaults: map[string]string{"mode": "idle"}}
	got := s.loadState("missing-key", config)
	if got["mode"] != "idle" {
		t.Errorf("expected mode=idle, got %q", got["mode"])
	}
}

func TestLoadState_StoredOverridesDefaults(t *testing.T) {
	// C3: stored state overrides defaults
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	s.storeState("s", "k1", map[string]string{"mode": "active", "extra": "x"})
	config := &suites.MockState{Defaults: map[string]string{"mode": "idle", "base": "val"}}
	got := s.loadState("k1", config)
	if got["mode"] != "active" {
		t.Errorf("stored value should override default: got %q", got["mode"])
	}
	if got["base"] != "val" {
		t.Errorf("default not in stored state should be present: got %q", got["base"])
	}
	if got["extra"] != "x" {
		t.Errorf("stored extra field should be present: got %q", got["extra"])
	}
}

func TestApplyStateTransition_NilConfig(t *testing.T) {
	// C4: nil config → no-op, result unchanged
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	result := &Result{Headers: make(http.Header)}
	s.applyStateTransition(nil, "key", "example", suites.Definition{ID: "s"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, nil, result)
	if result.Headers.Get("X-Babelsuite-State-Key") != "" {
		t.Error("expected no state key header set")
	}
}

func TestApplyStateTransition_TransitionFires(t *testing.T) {
	// C5: transition matches example name → state stored, header set
	t.Parallel()
	s := newTestService(suites.Definition{ID: "suite-1"})
	config := &suites.MockState{
		Transitions: []suites.MockStateTransition{
			{OnExample: "created", Set: map[string]string{"status": "active"}},
		},
	}
	result := &Result{Headers: make(http.Header)}
	s.applyStateTransition(config, "state-key", "created", suites.Definition{ID: "suite-1"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, nil, result)
	if result.Headers.Get("X-Babelsuite-State-Key") != "state-key" {
		t.Errorf("expected state key header, got %q", result.Headers.Get("X-Babelsuite-State-Key"))
	}
	loaded := s.loadState("state-key", config)
	if loaded["status"] != "active" {
		t.Errorf("expected status=active, got %q", loaded["status"])
	}
}

func TestApplyStateTransition_ExampleMismatch(t *testing.T) {
	// C7: OnExample mismatch → transition skipped, state unchanged
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	config := &suites.MockState{
		Transitions: []suites.MockStateTransition{
			{OnExample: "other-example", Set: map[string]string{"status": "active"}},
		},
	}
	result := &Result{Headers: make(http.Header)}
	s.applyStateTransition(config, "key", "different-example", suites.Definition{ID: "s"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, nil, result)
	if result.Headers.Get("X-Babelsuite-State-Key") != "" {
		t.Error("expected no state key header")
	}
}

func TestApplyStateTransition_DeleteField(t *testing.T) {
	// C10: Delete removes a field from state
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	initial := map[string]string{"count": "5", "other": "val"}
	config := &suites.MockState{
		Transitions: []suites.MockStateTransition{
			{Delete: []string{"count"}},
		},
	}
	result := &Result{Headers: make(http.Header)}
	s.applyStateTransition(config, "del-key", "", suites.Definition{ID: "s"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, initial, result)
	loaded := s.loadState("del-key", &suites.MockState{})
	if _, exists := loaded["count"]; exists {
		t.Error("count field should have been deleted")
	}
	if loaded["other"] != "val" {
		t.Errorf("other field should survive delete, got %q", loaded["other"])
	}
}

func TestApplyStateTransition_IncrementAbsentField(t *testing.T) {
	// C12: Increment on absent field → stored as "1"
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	config := &suites.MockState{
		Transitions: []suites.MockStateTransition{
			{Increment: map[string]int{"counter": 1}},
		},
	}
	result := &Result{Headers: make(http.Header)}
	s.applyStateTransition(config, "inc-key", "", suites.Definition{ID: "s"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, nil, result)
	loaded := s.loadState("inc-key", &suites.MockState{})
	if loaded["counter"] != "1" {
		t.Errorf("expected counter=1, got %q", loaded["counter"])
	}
}

func TestResetSuiteState_ClearsAllKeys(t *testing.T) {
	// C13: ResetSuiteState deletes all keys registered to the suite
	t.Parallel()
	suite := suites.Definition{ID: "suite-reset"}
	s := newTestService(suite)
	s.storeState("suite-reset", "k1", map[string]string{"a": "1"})
	s.storeState("suite-reset", "k2", map[string]string{"b": "2"})

	if err := s.ResetSuiteState(context.Background(), "suite-reset"); err != nil {
		t.Fatalf("ResetSuiteState: %v", err)
	}

	s.mu.RLock()
	_, k1exists := s.state["k1"]
	_, k2exists := s.state["k2"]
	s.mu.RUnlock()

	if k1exists || k2exists {
		t.Error("expected state cleared after reset")
	}
}

func TestApplyStateTransition_ConcurrentAccess(t *testing.T) {
	// C14: concurrent loadState + storeState → no data race (run with go test -race)
	t.Parallel()
	s := newTestService(suites.Definition{ID: "s"})
	config := &suites.MockState{
		Transitions: []suites.MockStateTransition{
			{Set: map[string]string{"k": "v"}},
		},
	}
	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			result := &Result{Headers: make(http.Header)}
			s.applyStateTransition(config, "race-key", "", suites.Definition{ID: "s"}, suites.APISurface{}, suites.APIOperation{}, requestSnapshot{}, nil, result)
			_ = s.loadState("race-key", config)
		}()
	}
	wg.Wait()
}
