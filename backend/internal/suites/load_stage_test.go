package suites

import (
	"strings"
	"testing"
	"time"
)

func TestParseLoadStage_RampUp(t *testing.T) {
	// C1: ramp-up stage
	t.Parallel()
	got, err := parseLoadStage(`duration="30s", users=10, spawn_rate=2`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: 30 * time.Second, Users: 10, SpawnRate: 2.0, Stop: false}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_StopStage(t *testing.T) {
	// C3: stop stage
	t.Parallel()
	got, err := parseLoadStage(`duration="30s", stop=true`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: 30 * time.Second, Users: 0, SpawnRate: 0, Stop: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_PositionalDuration(t *testing.T) {
	// C4: duration positional
	t.Parallel()
	got, err := parseLoadStage(`"1m", users=5`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: time.Minute, Users: 5, SpawnRate: 0, Stop: false}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_MissingDuration(t *testing.T) {
	// C6: missing duration
	t.Parallel()
	_, err := parseLoadStage(`users=10`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "requires duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseLoadStage_UnparseableDuration(t *testing.T) {
	// C7: unparseable duration
	t.Parallel()
	_, err := parseLoadStage(`duration="two minutes"`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid traffic stage duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseLoadStage_NegativeDuration(t *testing.T) {
	// C8: negative duration
	t.Parallel()
	_, err := parseLoadStage(`duration="-10s"`)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseLoadStage_C2_SteadyState(t *testing.T) {
	// C2: steady-state
	t.Parallel()
	got, err := parseLoadStage(`duration="2m", users=10`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: 2 * time.Minute, Users: 10}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_C5_NamedDurationOnly(t *testing.T) {
	// C5: duration named only
	t.Parallel()
	got, err := parseLoadStage(`duration="45s"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: 45 * time.Second}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_C9_ZeroUsers(t *testing.T) {
	// C9: zero users (hold current)
	t.Parallel()
	got, err := parseLoadStage(`duration="1m", users=0`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := LoadStage{Duration: time.Minute, Users: 0}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLoadStage_NegativeDurationRejectedAtBoundary(t *testing.T) {
	t.Parallel()
	d, err := time.ParseDuration("-1s")
	if err != nil || d >= 0 {
		t.Fatalf("test premise broken: stdlib ParseDuration(\"-1s\") err=%v d=%v", err, d)
	}
	_, parseErr := parseLoadStage(`duration="-1s"`)
	if parseErr == nil {
		t.Fatal("parseLoadStage accepted negative duration — post-condition Duration>0 violated")
	}
}

func TestParseLoadStage_NamedBeforePositional(t *testing.T) {
	t.Parallel()
	got, err := parseLoadStage(`"10s", duration="45s"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Duration != 45*time.Second {
		t.Errorf("named arg should win: got %v, want 45s", got.Duration)
	}
}

func TestValidateLoadSpec_MaxLoadDurationEnforced(t *testing.T) {
	t.Parallel()
	spec := &LoadSpec{
		PlanPath: "test/plan.star",
		Target:   "http://localhost:8080",
		Users: []LoadUser{{
			Name:  "u1",
			Tasks: []LoadTask{{Name: "t1", Request: LoadRequest{Method: "GET", Path: "/"}}},
		}},
		Stages: []LoadStage{
			{Duration: 3 * time.Minute, Users: 1},
			{Duration: 3 * time.Minute, Users: 1},
		},
	}
	err := validateLoadSpec(spec)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duration safety limit") {
		t.Errorf("unexpected error: %v", err)
	}
}
