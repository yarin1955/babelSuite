package examplefs

import (
	"path/filepath"
	"testing"
)

func TestResolveRootFromRepoUsesEnvOverride(t *testing.T) {
	t.Setenv(RootEnvVar, filepath.Join("custom", "examples"))

	root := ResolveRootFromRepo(filepath.Join("repo", "root"))
	expected, err := filepath.Abs(filepath.Join("custom", "examples"))
	if err != nil {
		t.Fatalf("abs expected: %v", err)
	}
	if root != expected {
		t.Fatalf("expected %q, got %q", expected, root)
	}
}
