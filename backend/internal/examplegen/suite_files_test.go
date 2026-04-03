package examplegen

import (
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestRenderMockSourceUsesDeclarativeGenerationRules(t *testing.T) {
	service := suites.NewService()
	suite, err := service.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	content := renderMockSource(*suite, "mock/returns/create-return.cue")

	if !strings.Contains(content, `@gen(`) {
		t.Fatalf("expected declarative generation rules, got %s", content)
	}
	if strings.Contains(content, "mockUUID") || strings.Contains(content, "mockNow") || strings.Contains(content, "mockInt") {
		t.Fatalf("expected generated schema to hide template helper calls, got %s", content)
	}
}

func TestRenderMockSourceUsesDeclarativeResolveRules(t *testing.T) {
	service := suites.NewService()
	suite, err := service.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	content := renderMockSource(*suite, "mock/returns/get-return.cue")

	if !strings.Contains(content, `@resolve(`) {
		t.Fatalf("expected declarative resolve rules, got %s", content)
	}
	if strings.Contains(content, "{{ request.path.returnId }}") || strings.Contains(content, "{{ state.status }}") {
		t.Fatalf("expected generated schema to hide template path expressions, got %s", content)
	}
}

func TestRenderMockSourceUsesDeclarativeComposeRulesForSOAP(t *testing.T) {
	service := suites.NewService()
	suite, err := service.Get("soap-claims-hub")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	content := renderMockSource(*suite, "mock/claims/claim-service.cue")

	if !strings.Contains(content, `@compose(`) {
		t.Fatalf("expected composed string rules for soap xml, got %s", content)
	}
	if strings.Contains(content, "mockUUID") || strings.Contains(content, "mockNow") || strings.Contains(content, "mockInt") {
		t.Fatalf("expected generated soap schema to hide template helper calls, got %s", content)
	}
}
