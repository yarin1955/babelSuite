package suites

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babelsuite/babelsuite/internal/demofs"
	"github.com/babelsuite/babelsuite/internal/examplefs"
)

func TestResolveTopologyExpandsNestedSuiteDependencies(t *testing.T) {
	t.Parallel()

	child := Definition{
		ID:         "auth-suite",
		Title:      "Auth Suite",
		Repository: "localhost:5000/core/auth-suite",
		Version:    "workspace",
		SuiteStar: strings.Join([]string{
			`db = container.run(name="db")`,
			`stub = mock.serve(name="stub", after=["db"])`,
			`api = container.run(name="api", after=["stub"])`,
		}, "\n"),
		Profiles: []ProfileOption{
			{FileName: "local.yaml", Default: true},
		},
		SourceFiles: []SourceFile{
			{
				Path: "profiles/local.yaml",
				Content: strings.TrimSpace(`
name: Local
default: true
env:
  JWT_AUDIENCE: payments
services:
  api:
    env:
      API_MODE: strict
`),
			},
		},
	}

	parent := Definition{
		ID:         "parent-suite",
		Title:      "Parent Suite",
		Repository: "localhost:5000/core/parent-suite",
		Version:    "workspace",
		SuiteStar: strings.Join([]string{
			`global = container.run(name="global-db")`,
			`auth = suite.run(ref="auth-module", after=["global-db"])`,
			`smoke = scenario.go(name="smoke", after=["auth"])`,
		}, "\n"),
		SourceFiles: []SourceFile{
			{
				Path: "dependencies.yaml",
				Content: strings.TrimSpace(`
dependencies:
  auth-module:
    ref: localhost:5000/core/auth-suite
    version: workspace
    profile: local.yaml
    inputs:
      DATABASE_URL: postgres://postgres:postgres@global-db:5432/auth
      REDIS_ADDR: redis:6379
`),
			},
			{
				Path: "dependencies.lock.yaml",
				Content: strings.TrimSpace(`
locks:
  auth-module:
    resolved: localhost:5000/core/auth-suite@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`),
			},
		},
	}

	topology, err := ResolveTopology(parent, []Definition{parent, child})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}

	if len(topology) != 5 {
		t.Fatalf("expected 5 resolved nodes, got %d", len(topology))
	}

	byID := map[string]TopologyNode{}
	for _, node := range topology {
		byID[node.ID] = node
	}

	if _, ok := byID["auth/db"]; !ok {
		t.Fatal("expected imported auth/db node")
	}
	if _, ok := byID["auth/api"]; !ok {
		t.Fatal("expected imported auth/api node")
	}
	if _, ok := byID["auth/stub"]; !ok {
		t.Fatal("expected imported auth/stub node")
	}
	if !containsString(byID["auth/db"].DependsOn, "global-db") {
		t.Fatalf("expected auth/db to depend on global-db, got %+v", byID["auth/db"].DependsOn)
	}
	if !containsString(byID["smoke"].DependsOn, "auth/api") {
		t.Fatalf("expected smoke to depend on imported auth/api exit node, got %+v", byID["smoke"].DependsOn)
	}
	if byID["auth/api"].RuntimeProfile != "local.yaml" {
		t.Fatalf("expected imported runtime profile local.yaml, got %q", byID["auth/api"].RuntimeProfile)
	}
	if got := byID["auth/api"].RuntimeEnv["DATABASE_URL"]; got != "postgres://postgres:postgres@global-db:5432/auth" {
		t.Fatalf("expected dependency input DATABASE_URL, got %q", got)
	}
	if got := byID["auth/api"].RuntimeEnv["JWT_AUDIENCE"]; got != "payments" {
		t.Fatalf("expected profile env JWT_AUDIENCE, got %q", got)
	}
	if got := byID["auth/api"].RuntimeEnv["API_MODE"]; got != "strict" {
		t.Fatalf("expected service-specific env API_MODE, got %q", got)
	}
	if got := byID["auth/stub"].RuntimeHeaders["x-suite-profile"]; got != "local.yaml" {
		t.Fatalf("expected mock runtime header x-suite-profile, got %q", got)
	}
	if got := byID["auth/api"].ResolvedRef; got != "localhost:5000/core/auth-suite@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected locked resolved ref, got %q", got)
	}
	if got := byID["auth/api"].Digest; got != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected digest metadata, got %q", got)
	}
}

func TestResolveTopologyRejectsMissingDependencyAlias(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:        "broken-suite",
		Title:     "Broken Suite",
		SuiteStar: `auth = suite.run(ref="missing-module")`,
	}

	_, err := ResolveTopology(suite, []Definition{suite})
	if err == nil || !strings.Contains(err.Error(), `missing dependency alias "missing-module"`) {
		t.Fatalf("expected missing dependency alias error, got %v", err)
	}
}

func TestResolveTopologyRejectsLatestDependencyTag(t *testing.T) {
	t.Parallel()

	child := Definition{
		ID:         "auth-suite",
		Title:      "Auth Suite",
		Repository: "localhost:5000/core/auth-suite",
		Version:    "workspace",
		SuiteStar:  `db = container.run(name="db")`,
	}

	parent := Definition{
		ID:        "parent-suite",
		Title:     "Parent Suite",
		SuiteStar: `auth = suite.run(ref="auth-module")`,
		SourceFiles: []SourceFile{
			{
				Path:    "dependencies.yaml",
				Content: "dependencies:\n  auth-module: \"localhost:5000/core/auth-suite:latest\"\n",
			},
		},
	}

	_, err := ResolveTopology(parent, []Definition{parent, child})
	if err == nil || !strings.Contains(err.Error(), "must use a pinned version instead of latest") {
		t.Fatalf("expected latest validation error, got %v", err)
	}
}

func TestResolveTopologyAcceptsLockedDigestWithoutManifestVersion(t *testing.T) {
	t.Parallel()

	child := Definition{
		ID:         "auth-suite",
		Title:      "Auth Suite",
		Repository: "localhost:5000/core/auth-suite",
		Version:    "workspace",
		SuiteStar:  `db = container.run(name="db")`,
	}

	parent := Definition{
		ID:        "parent-suite",
		Title:     "Parent Suite",
		SuiteStar: `auth = suite.run(ref="auth-module")`,
		SourceFiles: []SourceFile{
			{
				Path: strings.TrimSpace("dependencies.yaml"),
				Content: strings.TrimSpace(`
dependencies:
  auth-module:
    ref: localhost:5000/core/auth-suite
`),
			},
			{
				Path: strings.TrimSpace("dependencies.lock.yaml"),
				Content: strings.TrimSpace(`
locks:
  auth-module:
    version: workspace
    resolved: localhost:5000/core/auth-suite@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
`),
			},
		},
	}

	topology, err := ResolveTopology(parent, []Definition{parent, child})
	if err != nil {
		t.Fatalf("expected lockfile pin to resolve dependency, got %v", err)
	}
	if len(topology) != 1 {
		t.Fatalf("expected single imported node, got %d", len(topology))
	}
	if got := topology[0].ResolvedRef; got != "localhost:5000/core/auth-suite@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("expected resolved ref from lock file, got %q", got)
	}
}

func TestWorkspaceSuitesExposeRootDependencyManifestSource(t *testing.T) {
	root := t.TempDir()
	suiteRoot := filepath.Join(root, "oci-suites", "composite-suite")
	mustWriteFile(t, filepath.Join(suiteRoot, "suite.star"), `api = container.run(name="api")`)
	mustWriteFile(t, filepath.Join(suiteRoot, "README.md"), "# Composite Suite\n\nNested suite workspace.")
	mustWriteFile(t, filepath.Join(suiteRoot, "dependencies.yaml"), strings.TrimSpace(`
dependencies:
  auth-module:
    ref: localhost:5000/core/auth-suite
    version: workspace
`))
	mustWriteFile(t, filepath.Join(suiteRoot, "dependencies.lock.yaml"), strings.TrimSpace(`
locks:
  auth-module:
    resolved: localhost:5000/core/auth-suite@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
`))
	mustWriteFile(t, filepath.Join(suiteRoot, "profiles", "local.yaml"), strings.TrimSpace(`
name: Local
description: Local profile
default: true
runtime:
  suite: composite-suite
  repository: localhost:5000/core/composite-suite
  profileFile: local.yaml
`))

	t.Setenv(examplefs.RootEnvVar, root)
	t.Setenv(demofs.EnableEnvVar, "false")

	service := NewService()
	suite, err := service.Get("composite-suite")
	if err != nil {
		t.Fatalf("get workspace suite: %v", err)
	}

	for _, file := range suite.SourceFiles {
		if file.Path == "dependencies.yaml" {
			if !strings.Contains(file.Content, "version: workspace") {
				t.Fatalf("expected dependency alias in manifest, got %q", file.Content)
			}
			continue
		}
		if file.Path == "dependencies.lock.yaml" {
			if !strings.Contains(file.Content, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") {
				t.Fatalf("expected dependency lockfile digest, got %q", file.Content)
			}
			return
		}
	}

	t.Fatal("expected dependency manifest and lock file to be exposed as root source files")
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
