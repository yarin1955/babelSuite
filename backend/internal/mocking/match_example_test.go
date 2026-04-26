package mocking

import (
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestMatchExample_SingleUnconditional(t *testing.T) {
	// C1: single unconditional exchange (fast path)
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "default", ResponseBody: "ok"},
		},
	}
	got, ok := matchExample(op, requestSnapshot{})
	if !ok || got.Name != "default" {
		t.Errorf("want default/true, got %q/%v", got.Name, ok)
	}
}

func TestMatchExample_MultipleFirstMatches(t *testing.T) {
	// C2: multiple exchanges, first When matches
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "first", When: []suites.MatchCondition{{From: "query", Param: "status", Value: "active"}}},
			{Name: "second", When: []suites.MatchCondition{{From: "query", Param: "status", Value: "inactive"}}},
		},
	}
	got, ok := matchExample(op, requestSnapshot{Query: map[string]string{"status": "active"}})
	if !ok || got.Name != "first" {
		t.Errorf("want first/true, got %q/%v", got.Name, ok)
	}
}

func TestMatchExample_NoMatch(t *testing.T) {
	// C4: multiple exchanges, neither When matches
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "a", When: []suites.MatchCondition{{From: "query", Param: "x", Value: "1"}}},
			{Name: "b", When: []suites.MatchCondition{{From: "query", Param: "x", Value: "2"}}},
		},
	}
	_, ok := matchExample(op, requestSnapshot{Query: map[string]string{"x": "3"}})
	if ok {
		t.Error("expected false, got true")
	}
}

func TestMatchExample_EmptyExchanges(t *testing.T) {
	// C5: empty exchanges list — no panic, returns false (proof Q2)
	t.Parallel()
	_, ok := matchExample(suites.APIOperation{}, requestSnapshot{})
	if ok {
		t.Error("expected false for empty exchanges, got true")
	}
}

func TestMatchExample_SingleConditionalSatisfied(t *testing.T) {
	// C6: single exchange with When, condition satisfied
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "conditional", When: []suites.MatchCondition{{From: "query", Param: "v", Value: "yes"}}},
		},
	}
	got, ok := matchExample(op, requestSnapshot{Query: map[string]string{"v": "yes"}})
	if !ok || got.Name != "conditional" {
		t.Errorf("want conditional/true, got %q/%v", got.Name, ok)
	}
}

func TestMatchExample_SingleConditionalFails(t *testing.T) {
	// C7: single exchange with When, condition fails
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "conditional", When: []suites.MatchCondition{{From: "query", Param: "v", Value: "yes"}}},
		},
	}
	_, ok := matchExample(op, requestSnapshot{Query: map[string]string{"v": "no"}})
	if ok {
		t.Error("expected false, got true")
	}
}

func TestMatchExample_HeaderCaseInsensitive(t *testing.T) {
	// C9: header key mixed-case in When, snapshot has lowercase key
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{
				Name: "json-accept",
				When: []suites.MatchCondition{{From: "header", Param: "Content-Type", Value: "application/json"}},
			},
		},
	}
	snap := requestSnapshot{Headers: map[string]string{"content-type": "application/json"}}
	got, ok := matchExample(op, snap)
	if !ok || got.Name != "json-accept" {
		t.Errorf("want json-accept/true, got %q/%v", got.Name, ok)
	}
}

func TestMatchExample_LastUnconditionalFallback(t *testing.T) {
	// C12: last exchange has empty When (unconditional fallback after failed conditions)
	t.Parallel()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "specific", When: []suites.MatchCondition{{From: "query", Param: "x", Value: "1"}}},
			{Name: "fallback"},
		},
	}
	got, ok := matchExample(op, requestSnapshot{Query: map[string]string{"x": "999"}})
	if !ok || got.Name != "fallback" {
		t.Errorf("want fallback/true, got %q/%v", got.Name, ok)
	}
}
