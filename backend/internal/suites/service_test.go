package suites

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/babelsuite/babelsuite/internal/examplefs"
)

func TestGetReturnsClonedSuite(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	suite.Profiles[0].FileName = "mutated.yaml"
	suite.SourceFiles[0].Content = "mutated"

	reloaded, err := service.Get("payment-suite")
	if err != nil {
		t.Fatalf("get suite again: %v", err)
	}
	if reloaded.Profiles[0].FileName != "local.yaml" {
		t.Fatalf("expected original profile to be preserved, got %q", reloaded.Profiles[0].FileName)
	}
	if reloaded.SourceFiles[0].Content == "mutated" {
		t.Fatal("expected source files to be cloned")
	}
}

func TestListReturnsSortedSuites(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	items := service.List()
	if len(items) != 5 {
		t.Fatalf("expected 5 suites, got %d", len(items))
	}
	if items[0].Title != "Fleet Control Room" {
		t.Fatalf("expected sorted suites, got %q first", items[0].Title)
	}
	if items[len(items)-1].Title != "Storefront Browser Lab" {
		t.Fatalf("expected storefront browser lab last, got %q", items[len(items)-1].Title)
	}
}

func TestStorefrontSuiteHydratesSourceFiles(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("storefront-browser-lab")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "profiles/local.yaml" {
			continue
		}
		found = true
		if file.Language != "yaml" {
			t.Fatalf("expected yaml language, got %q", file.Language)
		}
		if file.Content == "" {
			t.Fatal("expected hydrated source file content")
		}
	}

	if !found {
		t.Fatal("expected storefront source files to include profiles/local.yaml")
	}
}

func TestReturnsSuiteHydratesMockMetadataFiles(t *testing.T) {
	configureExamplesRoot(t)

	service := NewService()

	suite, err := service.Get("returns-control-plane")
	if err != nil {
		t.Fatalf("get suite: %v", err)
	}

	var found bool
	for _, file := range suite.SourceFiles {
		if file.Path != "mock/returns/create-return.metadata.yaml" {
			continue
		}
		found = true
		if file.Language != "yaml" {
			t.Fatalf("expected yaml language, got %q", file.Language)
		}
		if file.Content == "" {
			t.Fatal("expected hydrated metadata content")
		}
	}

	if !found {
		t.Fatal("expected returns suite source files to include mock metadata")
	}
}

func configureExamplesRoot(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	t.Setenv(examplefs.RootEnvVar, filepath.Join(repoRoot, "examples"))
}
