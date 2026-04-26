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
			`db = service.run()`,
			`stub = service.mock(after=[db])`,
			`api = service.run(after=[stub])`,
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
			`global_db = service.run(id="global-db")`,
			`auth = suite.run(ref="auth-module", after=[global_db])`,
			`smoke = test.run(file="smoke_test.py", image="python:3.12", after=[auth])`,
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

func TestResolveTopologyDefaultsNodeIDToAssignmentAndSupportsIdentifierDependencies(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:        "implicit-suite",
		Title:     "Implicit Suite",
		SuiteStar: strings.Join([]string{`db = service.run()`, `seed_data = task.run(file="seed.sh", image="bash:5.2", after=[db])`, `smoke_test = test.run(file="smoke_test.py", image="python:3.12", after=[seed_data])`}, "\n"),
	}

	topology, err := ResolveTopology(suite, []Definition{suite})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 resolved nodes, got %d", len(topology))
	}
	if topology[0].ID != "db" || topology[1].ID != "seed_data" || topology[2].ID != "smoke_test" {
		t.Fatalf("expected assignment names to become ids, got %q, %q, %q", topology[0].ID, topology[1].ID, topology[2].ID)
	}
	if got := topology[1].DependsOn; len(got) != 1 || got[0] != "db" {
		t.Fatalf("expected seed_data to depend on db, got %v", got)
	}
	if got := topology[2].DependsOn; len(got) != 1 || got[0] != "seed_data" {
		t.Fatalf("expected smoke_test to depend on seed_data, got %v", got)
	}
}

func TestResolveTopologyParsesArtifactExportChains(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:    "artifact-suite",
		Title: "Artifact Suite",
		SuiteStar: strings.Join([]string{
			`smoke = test.run(file="smoke_test.py", image="python:3.12") \`,
			`  .export("coverage/*.xml", name="go-coverage", on="always", format="cobertura") \`,
			`  .export("logs/crash.dump", name="crash-debug", on="failure")`,
		}, "\n"),
	}

	topology, err := ResolveTopology(suite, []Definition{suite})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}
	if len(topology) != 1 {
		t.Fatalf("expected 1 resolved node, got %d", len(topology))
	}
	if topology[0].ID != "smoke" {
		t.Fatalf("expected smoke node id, got %q", topology[0].ID)
	}
	if len(topology[0].ArtifactExports) != 2 {
		t.Fatalf("expected 2 artifact exports, got %d", len(topology[0].ArtifactExports))
	}
	if got := topology[0].ArtifactExports[0]; got.Path != "coverage/*.xml" || got.Name != "go-coverage" || got.On != "always" || got.Format != "cobertura" {
		t.Fatalf("unexpected first artifact export: %+v", got)
	}
	if got := topology[0].ArtifactExports[1]; got.Path != "logs/crash.dump" || got.Name != "crash-debug" || got.On != "failure" {
		t.Fatalf("unexpected second artifact export: %+v", got)
	}
}

func TestResolveTopologyParsesStepEvaluationControls(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:    "evaluation-suite",
		Title: "Evaluation Suite",
		SuiteStar: strings.Join([]string{
			`seed = task.run(file="seed.sh", image="bash:5.2", expect_exit=0, expect_logs=["Task completed successfully"], fail_on_logs="FATAL ERROR")`,
			`smoke = test.run(file="smoke_test.py", image="python:3.12", after=[seed], expect_exit=0, expect_logs="Test checks completed")`,
		}, "\n"),
	}

	topology, err := ResolveTopology(suite, []Definition{suite})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}
	if len(topology) != 2 {
		t.Fatalf("expected 2 resolved nodes, got %d", len(topology))
	}
	if topology[0].Evaluation == nil || topology[0].Evaluation.ExpectExit == nil || *topology[0].Evaluation.ExpectExit != 0 {
		t.Fatalf("expected seed expect_exit=0, got %+v", topology[0].Evaluation)
	}
	if len(topology[0].Evaluation.ExpectLogs) != 1 || topology[0].Evaluation.ExpectLogs[0] != "Task completed successfully" {
		t.Fatalf("expected seed expect_logs to round-trip, got %+v", topology[0].Evaluation)
	}
	if len(topology[0].Evaluation.FailOnLogs) != 1 || topology[0].Evaluation.FailOnLogs[0] != "FATAL ERROR" {
		t.Fatalf("expected seed fail_on_logs to round-trip, got %+v", topology[0].Evaluation)
	}
	if topology[1].Evaluation == nil || len(topology[1].Evaluation.ExpectLogs) != 1 || topology[1].Evaluation.ExpectLogs[0] != "Test checks completed" {
		t.Fatalf("expected smoke expect_logs to round-trip, got %+v", topology[1].Evaluation)
	}
}

func TestResolveTopologyParsesFailureFlowControls(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:    "failure-flow-suite",
		Title: "Failure Flow Suite",
		SuiteStar: strings.Join([]string{
			`billing_mock = service.mock()`,
			`lint = test.run(file="lint.sh", image="bash:5.2", expect_logs="THIS STRING DOES NOT EXIST", continue_on_failure=true, reset_mocks=[billing_mock])`,
			`rollback = task.run(file="rollback.sh", image="bash:5.2", on_failure=[lint])`,
		}, "\n"),
	}

	topology, err := ResolveTopology(suite, []Definition{suite})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 resolved nodes, got %d", len(topology))
	}
	if !topology[1].ContinueOnFailure {
		t.Fatalf("expected lint to enable continue_on_failure, got %+v", topology[1])
	}
	if len(topology[1].ResetMocks) != 1 || topology[1].ResetMocks[0] != "billing_mock" {
		t.Fatalf("expected lint reset_mocks billing_mock, got %+v", topology[1].ResetMocks)
	}
	if len(topology[2].OnFailure) != 1 || topology[2].OnFailure[0] != "lint" {
		t.Fatalf("expected rollback on_failure lint, got %+v", topology[2].OnFailure)
	}
	if len(topology[2].DependsOn) != 1 || topology[2].DependsOn[0] != "lint" {
		t.Fatalf("expected rollback dependsOn lint, got %+v", topology[2].DependsOn)
	}
}

func TestResolveTopologyParsesNativeTrafficSpec(t *testing.T) {
	t.Parallel()

	suite := Definition{
		ID:        "traffic-suite",
		Title:     "Traffic Suite",
		SuiteStar: `baseline = traffic.baseline(plan="smoke.star", target="http://127.0.0.1:18080")`,
		SourceFiles: []SourceFile{
			{
				Path: "traffic/smoke.star",
				Content: strings.TrimSpace(`
load("@babelsuite/runtime", "traffic")

probe = traffic.user(
    name="probe",
    wait=traffic.constant(0),
    tasks=[
        traffic.task(
            name="health",
            request=traffic.get("/health", name="health"),
            checks=[
                traffic.threshold("status", "==", 200),
                traffic.threshold("latency.p95_ms", "<", 500),
            ],
        ),
    ],
)

traffic.plan(
    users=[probe],
    shape=traffic.stages([
        traffic.stage(duration="1s", users=2, spawn_rate=1),
    ]),
    thresholds=[
        traffic.threshold("http.error_rate", "<=", 0),
    ],
)
`),
			},
		},
	}

	topology, err := ResolveTopology(suite, []Definition{suite})
	if err != nil {
		t.Fatalf("resolve topology: %v", err)
	}
	if len(topology) != 1 {
		t.Fatalf("expected 1 topology node, got %d", len(topology))
	}
	if topology[0].Load == nil {
		t.Fatal("expected parsed traffic spec")
	}
	if topology[0].Load.PlanPath != "traffic/smoke.star" {
		t.Fatalf("expected normalized plan path, got %q", topology[0].Load.PlanPath)
	}
	if topology[0].Load.Target != "http://127.0.0.1:18080" {
		t.Fatalf("expected target to round-trip, got %q", topology[0].Load.Target)
	}
	if len(topology[0].Load.Users) != 1 {
		t.Fatalf("expected 1 traffic user, got %d", len(topology[0].Load.Users))
	}
	if len(topology[0].Load.Stages) != 1 {
		t.Fatalf("expected 1 traffic stage, got %d", len(topology[0].Load.Stages))
	}
	if len(topology[0].Load.Thresholds) != 1 {
		t.Fatalf("expected 1 plan threshold, got %d", len(topology[0].Load.Thresholds))
	}
	if topology[0].Load.Users[0].Tasks[0].Request.Method != "GET" {
		t.Fatalf("expected GET request, got %q", topology[0].Load.Users[0].Tasks[0].Request.Method)
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
		SuiteStar:  `db = service.run(name="db")`,
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
		SuiteStar:  `db = service.run(name="db")`,
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
	mustWriteFile(t, filepath.Join(suiteRoot, "suite.star"), `api = service.run(name="api")`)
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
