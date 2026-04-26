package mocking

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func newFallbackService() *Service {
	return &Service{
		suites:     staticSuiteReader{suite: suites.Definition{ID: "s"}},
		state:      make(map[string]map[string]string),
		suiteState: make(map[string]map[string]struct{}),
	}
}

func TestResolveFallback_NilFallback(t *testing.T) {
	// C1: no fallback configured → 404 "No matching mock exchange"
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 404 {
		t.Fatalf("expected 404, got %v", result)
	}
	if !strings.Contains(string(result.Body), "No matching mock exchange") {
		t.Errorf("unexpected body: %s", result.Body)
	}
}

func TestResolveFallback_ExampleModeFoundInExchanges(t *testing.T) {
	// C3: mode "example", named example found in Exchanges → rendered response
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		Exchanges: []suites.ExchangeExample{
			{Name: "get-order", ResponseStatus: "200", ResponseBody: `{"id":"1"}`},
		},
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{Mode: "example", ExampleName: "get-order"},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 200 {
		t.Fatalf("expected 200, got %v", result)
	}
	if result.MatchedExample != "get-order" {
		t.Errorf("expected MatchedExample=get-order, got %q", result.MatchedExample)
	}
}

func TestResolveFallback_ExampleModeNotFound(t *testing.T) {
	// C4: mode "example", name not found → 404 "Fallback example was not found"
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{Mode: "example", ExampleName: "missing"},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 404 {
		t.Fatalf("expected 404, got %v", result)
	}
	if !strings.Contains(string(result.Body), "Fallback example was not found") {
		t.Errorf("unexpected body: %s", result.Body)
	}
}

func TestResolveFallback_ProxyBlankURL(t *testing.T) {
	// C6: mode "proxy", blank ProxyURL → 502
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{Mode: "proxy", ProxyURL: ""},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 502 {
		t.Fatalf("expected 502, got %v", result)
	}
}

func TestResolveFallback_StaticModeWithBody(t *testing.T) {
	// C8: static mode with body and headers
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{
				Mode:   "static",
				Status: "201",
				Body:   "created",
				Headers: []suites.Header{
					{Name: "X-Custom", Value: "val"},
				},
			},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 201 {
		t.Fatalf("expected 201, got %v", result)
	}
	if string(result.Body) != "created" {
		t.Errorf("expected body 'created', got %s", result.Body)
	}
	if result.Headers.Get("X-Custom") != "val" {
		t.Errorf("expected X-Custom=val, got %q", result.Headers.Get("X-Custom"))
	}
}

func TestResolveFallback_StaticNoStatusDefaults400(t *testing.T) {
	// C9: static mode, no status → defaults to 400
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{Mode: "static", Body: "fallback"},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 400 {
		t.Fatalf("expected 400 default, got %v", result)
	}
}

func TestResolveFallback_MixedCaseModeNormalized(t *testing.T) {
	// C10: mode "Proxy" (mixed case) → normalised to proxy mode → 502 blank URL
	t.Parallel()
	s := newFallbackService()
	op := suites.APIOperation{
		MockMetadata: suites.MockOperationMetadata{
			Fallback: &suites.MockFallback{Mode: "Proxy", ProxyURL: ""},
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := s.resolveFallback(context.Background(), suites.Definition{}, suites.APISurface{}, op, requestSnapshot{}, nil, req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != 502 {
		t.Fatalf("expected 502 (proxy mode), got %v", result)
	}
}

func TestProxyFallback_NeverReturnsNilNil(t *testing.T) {
	// Proof Q1: proxyFallback never returns (nil, nil)
	// Blank ProxyURL path: returns non-nil *Result, nil error
	t.Parallel()
	fallback := &suites.MockFallback{ProxyURL: ""}
	req := httptest.NewRequest("GET", "/", nil)
	result, err := proxyFallback(context.Background(), req, fallback, "", "", "")
	if result == nil && err == nil {
		t.Fatal("proxyFallback returned (nil, nil) — invariant violated")
	}
	if result == nil {
		t.Fatal("expected non-nil result for blank URL case")
	}
}
