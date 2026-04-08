package createcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/cli/ctl/internal/support"
)

func TestRunCreatesStarterSuiteTemplate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	destination := filepath.Join(root, "starter-suite")
	rt := &support.Runtime{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}

	status := Run(context.Background(), rt, support.GlobalOptions{}, []string{"template", "starter-suite", destination})
	if status != 0 {
		t.Fatalf("expected exit 0, got %d", status)
	}

	expectedFiles := []string{
		"suite.star",
		"profiles/local.yaml",
		"api/openapi.yaml",
		"mock/catalog/get-item.cue",
		"mock/catalog/get-item.metadata.yaml",
		"scripts/bootstrap.sh",
		"load/http_smoke.star",
		"load/users.csv",
		"scenarios/http/smoke.hurl",
		"docs/README.md",
	}

	for _, relative := range expectedFiles {
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("expected %s to exist: %v", target, err)
		}
	}
}

func TestRunRejectsUnsafeOrEmptyTemplateName(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rt := &support.Runtime{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	status := Run(context.Background(), rt, support.GlobalOptions{}, []string{"template", "!!!"})
	if status == 0 {
		t.Fatal("expected non-zero status for invalid template name")
	}
	if !strings.Contains(stderr.String(), "template name") {
		t.Fatalf("expected template name error, got %q", stderr.String())
	}
}
