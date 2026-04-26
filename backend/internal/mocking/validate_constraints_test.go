package mocking

import (
	"net/http"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestValidateConstraints_NoConstraints(t *testing.T) {
	// C1: empty constraint list → nil
	t.Parallel()
	if got := validateConstraints(nil, requestSnapshot{}); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestValidateConstraints_RequiredPresent(t *testing.T) {
	// C2: required param present → nil
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "id", Source: "query", Required: true}}
	snap := requestSnapshot{Query: map[string]string{"id": "123"}}
	if got := validateConstraints(constraints, snap); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestValidateConstraints_RequiredAbsent(t *testing.T) {
	// C3: required param absent → 400 with "Missing required"
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "id", Source: "query", Required: true}}
	got := validateConstraints(constraints, requestSnapshot{Query: map[string]string{}})
	if got == nil || got.Status != 400 {
		t.Fatalf("expected 400 result, got %v", got)
	}
	if !strings.Contains(string(got.Body), "Missing required") {
		t.Errorf("body should contain 'Missing required', got %s", got.Body)
	}
}

func TestValidateConstraints_PatternMatches(t *testing.T) {
	// C4: optional param with pattern, value matches → nil
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "code", Source: "query", Pattern: `^\d{3}$`}}
	snap := requestSnapshot{Query: map[string]string{"code": "404"}}
	if got := validateConstraints(constraints, snap); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestValidateConstraints_PatternFails(t *testing.T) {
	// C5: optional param with pattern, value fails → 400 with "failed validation"
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "code", Source: "query", Pattern: `^\d{3}$`}}
	snap := requestSnapshot{Query: map[string]string{"code": "not-a-code"}}
	got := validateConstraints(constraints, snap)
	if got == nil || got.Status != 400 {
		t.Fatalf("expected 400 result, got %v", got)
	}
	if !strings.Contains(string(got.Body), "failed validation") {
		t.Errorf("body should contain 'failed validation', got %s", got.Body)
	}
}

func TestValidateConstraints_AbsentOptionalSkipped(t *testing.T) {
	// C6: absent optional param with pattern → skip, not error
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "code", Source: "query", Pattern: `^\d{3}$`}}
	if got := validateConstraints(constraints, requestSnapshot{}); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestValidateConstraints_InvalidRegexSkipped(t *testing.T) {
	// C7: invalid regex → skip, not error
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "x", Source: "query", Pattern: `[invalid`}}
	snap := requestSnapshot{Query: map[string]string{"x": "value"}}
	if got := validateConstraints(constraints, snap); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestValidateConstraints_ShortCircuitOnFirstFail(t *testing.T) {
	// C9: first constraint fails → 400 mentioning first param; second not evaluated
	t.Parallel()
	constraints := []suites.ParameterConstraint{
		{Name: "first", Source: "query", Required: true},
		{Name: "second", Source: "query", Required: true},
	}
	snap := requestSnapshot{Query: map[string]string{"second": "present"}}
	got := validateConstraints(constraints, snap)
	if got == nil || got.Status != 400 {
		t.Fatalf("expected 400 result, got %v", got)
	}
	if !strings.Contains(string(got.Body), `\"first\"`) {
		t.Errorf("error should name the first param, got %s", got.Body)
	}
}

func TestApplyRecopiedConstraintHeaders_Forward(t *testing.T) {
	// C10: Forward=true, value present → header set
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "X-Trace-Id", Source: "header", Forward: true}}
	snap := requestSnapshot{Headers: map[string]string{"x-trace-id": "abc123"}}
	headers := make(http.Header)
	applyRecopiedConstraintHeaders(headers, constraints, snap)
	if headers.Get("X-Trace-Id") != "abc123" {
		t.Errorf("expected X-Trace-Id=abc123, got %q", headers.Get("X-Trace-Id"))
	}
}

func TestApplyRecopiedConstraintHeaders_AbsentNotSet(t *testing.T) {
	// C11: Forward=true, value absent → header not set
	t.Parallel()
	constraints := []suites.ParameterConstraint{{Name: "X-Trace-Id", Source: "header", Forward: true}}
	headers := make(http.Header)
	applyRecopiedConstraintHeaders(headers, constraints, requestSnapshot{})
	if headers.Get("X-Trace-Id") != "" {
		t.Errorf("expected empty header, got %q", headers.Get("X-Trace-Id"))
	}
}
