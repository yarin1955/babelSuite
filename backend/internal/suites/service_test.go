package suites

import "testing"

func TestGetReturnsClonedSuite(t *testing.T) {
	service := NewService()

	suite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	suite.Profiles[0].FileName = "mutated.yaml"

	reloaded, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite again: %v", err)
	}
	if reloaded.Profiles[0].FileName != "local.yaml" {
		t.Fatalf("expected original profile to be preserved, got %q", reloaded.Profiles[0].FileName)
	}
}

func TestListReturnsSortedSuites(t *testing.T) {
	service := NewService()

	items := service.List()
	if len(items) != 3 {
		t.Fatalf("expected 3 suites, got %d", len(items))
	}
	if items[0].Title != "Fleet Control Room" {
		t.Fatalf("expected sorted suites, got %q first", items[0].Title)
	}
}
