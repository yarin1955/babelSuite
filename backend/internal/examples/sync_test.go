package examples

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/examplefs"
)

func TestWorkspaceExamplesMatchDefinitions(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	t.Setenv(examplefs.RootEnvVar, filepath.Join(repoRoot, "examples"))

	for _, file := range RenderWorkspaceFiles() {
		target := filepath.Join(examplefs.ResolveRoot(), filepath.FromSlash(file.Path))
		t.Run(file.Path, func(t *testing.T) {
			body, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read %s: %v", target, err)
			}

			if normalizeLineEndings(string(body)) != normalizeLineEndings(file.Content) {
				t.Fatalf("content mismatch for %s", target)
			}
		})
	}
}

func TestWorkspaceExamplesDoNotMaterializeGatewayArtifacts(t *testing.T) {
	for _, file := range RenderWorkspaceFiles() {
		if strings.Contains(filepath.ToSlash(file.Path), "/gateway/") {
			t.Fatalf("did not expect generated example workspace to include gateway artifact %s", file.Path)
		}
		if filepath.Base(file.Path) == "README.md" && strings.Contains(file.Content, "- `gateway/`:") {
			t.Fatalf("did not expect example README to list runtime-managed gateway folder in %s", file.Path)
		}
	}
}

func normalizeLineEndings(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
