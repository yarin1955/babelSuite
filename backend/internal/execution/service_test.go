package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/babelsuite/babelsuite/internal/agent"
	"github.com/babelsuite/babelsuite/internal/logstream"
	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"github.com/babelsuite/babelsuite/internal/runner"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type captureObserver struct {
	events chan Snapshot
}

type stubPlatformSource struct {
	settings *platform.PlatformSettings
	err      error
}

type stubRuntimeStore struct {
	executions []PersistedExecution
}

type stubMockResetter struct {
	suiteIDs []string
}

func (s stubPlatformSource) Load() (*platform.PlatformSettings, error) {
	return s.settings, s.err
}

func (s *stubRuntimeStore) LoadExecutionRuntime(_ context.Context) ([]PersistedExecution, error) {
	return append([]PersistedExecution{}, s.executions...), nil
}

func (s *stubRuntimeStore) SaveExecutionRuntime(_ context.Context, executions []PersistedExecution) error {
	s.executions = append([]PersistedExecution{}, executions...)
	return nil
}

func (s *stubMockResetter) ResetSuiteState(_ context.Context, suiteID string) error {
	s.suiteIDs = append(s.suiteIDs, suiteID)
	return nil
}

func (o *captureObserver) SyncExecution(snapshot Snapshot) {
	select {
	case o.events <- snapshot:
	default:
	}
}

func TestCreateExecutionUsesExplicitRemoteBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID:     "remote-agent",
					Name:        "Remote Worker",
					Type:        "remote-agent",
					Description: "Dispatches steps to a worker process.",
					Enabled:     true,
					Default:     true,
					HostURL:     "http://worker.example",
				},
			},
		},
	})
	defer service.Close()

	registry := agent.NewRegistry(nil)
	coordinator := agent.NewCoordinator(registry, service)
	service.ConfigureRemoteWorkers(registry, coordinator)

	controlPlane := httptest.NewServer(agent.NewGateway(registry, coordinator))
	defer controlPlane.Close()

	client := agent.NewControlPlaneClient(controlPlane.URL, controlPlane.Client())
	if err := client.Register(context.Background(), agent.RegisterRequest{
		AgentID: "remote-agent",
		Name:    "Remote Worker",
		HostURL: "http://worker.example",
	}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	workerService := agent.NewService(agent.Info{
		AgentID: "remote-agent",
		Name:    "Remote Worker",
		Status:  "ready",
	}, agent.ExecutorFunc(func(ctx context.Context, request agent.StepRequest, emit func(line logstream.Line)) error {
		emit(logstream.Line{Source: request.Node.ID, Level: "info", Text: "remote worker running"})
		return nil
	}))
	worker := agent.NewWorker("remote-agent", 20*time.Millisecond, client, workerService)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go func() { _ = worker.Run(workerCtx) }()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
		Backend: "remote-agent",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.BackendID != "remote-agent" {
		t.Fatalf("expected remote-agent backend id, got %q", execution.BackendID)
	}
	if execution.Backend != "Remote Worker" {
		t.Fatalf("expected backend label to round-trip, got %q", execution.Backend)
	}
}

func TestConfigureRuntimeStoreRestoresPersistedExecutions(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	store := &stubRuntimeStore{
		executions: []PersistedExecution{
			{
				Record: ExecutionRecord{
					ID:        "run-persisted",
					Suite:     ExecutionSuite{ID: "payment-suite", Title: "Payment Suite"},
					Profile:   "local.yaml",
					Backend:   "Remote Worker",
					Status:    "Booting",
					Trigger:   "Manual",
					StartedAt: time.Now().UTC().Add(-time.Minute),
					UpdatedAt: time.Now().UTC(),
				},
				Total:     3,
				Completed: 1,
				Logs: []logstream.Line{
					{Source: "db", Level: "info", Text: "restored"},
				},
			},
		},
	}

	service.ConfigureRuntimeStore(store)

	executions := service.ListExecutions()
	if len(executions) != 1 {
		t.Fatalf("expected one restored execution, got %d", len(executions))
	}
	if executions[0].ID != "run-persisted" {
		t.Fatalf("expected restored execution id, got %q", executions[0].ID)
	}

	record, err := service.GetExecution("run-persisted")
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if record.Backend != "Remote Worker" {
		t.Fatalf("expected restored backend, got %q", record.Backend)
	}
}

func TestCreateExecutionRejectsUnknownBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "local-docker",
					Name:    "Local Docker",
					Type:    "local",
					Enabled: true,
					Default: true,
				},
			},
		},
	})
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
		Backend: "missing-backend",
	})
	if !errors.Is(err, ErrBackendNotFound) {
		t.Fatalf("expected ErrBackendNotFound, got %v", err)
	}
}

func TestCreateExecutionRejectsUnavailableBackend(t *testing.T) {
	service := NewServiceWithPlatform(suites.NewService(), stubPlatformSource{
		settings: &platform.PlatformSettings{
			Agents: []platform.ExecutionAgent{
				{
					AgentID: "remote-agent",
					Name:    "Remote Worker",
					Type:    "remote-agent",
					Enabled: true,
					Default: true,
					HostURL: "http://127.0.0.1:1",
				},
			},
		},
	})
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "local.yaml",
	})
	if !errors.Is(err, ErrBackendUnavailable) {
		t.Fatalf("expected ErrBackendUnavailable, got %v", err)
	}
}

type staticSuiteSource struct {
	items map[string]suites.Definition
}

func (s staticSuiteSource) List() []suites.Definition {
	result := make([]suites.Definition, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result
}

func (s staticSuiteSource) Get(id string) (*suites.Definition, error) {
	item, ok := s.items[id]
	if !ok {
		return nil, suites.ErrNotFound
	}
	return &item, nil
}

func TestCreateExecutionRejectsProfileFromAnotherSuite(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "perf.yaml",
	})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestCreateExecutionUsesSuiteSpecificDefaultProfile(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.Profile != "local.yaml" {
		t.Fatalf("expected suite default profile, got %q", execution.Profile)
	}
}

func TestCreateExecutionRejectsInvalidTopology(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"broken-suite": {
				ID:        "broken-suite",
				Title:     "Broken Suite",
				SuiteStar: "api = service.run(name=\"api\", after=[\"db\"])\n",
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "broken-suite",
		Profile: "local.yaml",
	})
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected ErrInvalidTopology, got %v", err)
	}
}

func TestCreateExecutionExpandsNestedSuiteTopology(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"child-suite": {
				ID:         "child-suite",
				Title:      "Child Suite",
				Repository: "localhost:5000/core/child-suite",
				Version:    "workspace",
				SuiteStar: strings.Join([]string{
					`db = service.run(name="db")`,
					`stub = service.mock(name="stub", after=["db"])`,
					`api = service.run(name="api", after=["stub"])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
				SourceFiles: []suites.SourceFile{
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
			},
			"parent-suite": {
				ID:         "parent-suite",
				Title:      "Parent Suite",
				Repository: "localhost:5000/core/parent-suite",
				Version:    "workspace",
				SuiteStar: strings.Join([]string{
					`global = service.run(name="global-db")`,
					`auth = suite.run(ref="auth-module", after=["global-db"])`,
					`smoke = test.run(name="smoke", file="smoke_test.py", image="python:3.12", after=["auth"])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
				SourceFiles: []suites.SourceFile{
					{
						Path: "dependencies.yaml",
						Content: strings.TrimSpace(`
dependencies:
  auth-module:
    ref: localhost:5000/core/child-suite
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
    resolved: localhost:5000/core/child-suite@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
`),
					},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "parent-suite",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}

	if len(record.Suite.Topology) != 5 {
		t.Fatalf("expected 5 resolved topology nodes, got %d", len(record.Suite.Topology))
	}

	byID := map[string]suites.TopologyNode{}
	for _, node := range record.Suite.Topology {
		byID[node.ID] = node
	}

	if !containsTopologyDependency(byID["auth/db"].DependsOn, "global-db") {
		t.Fatalf("expected auth/db to depend on global-db, got %+v", byID["auth/db"].DependsOn)
	}
	if !containsTopologyDependency(byID["smoke"].DependsOn, "auth/api") {
		t.Fatalf("expected smoke to depend on auth/api, got %+v", byID["smoke"].DependsOn)
	}
	if byID["auth/api"].RuntimeProfile != "local.yaml" {
		t.Fatalf("expected local.yaml runtime profile, got %q", byID["auth/api"].RuntimeProfile)
	}
	if got := byID["auth/api"].RuntimeEnv["DATABASE_URL"]; got != "postgres://postgres:postgres@global-db:5432/auth" {
		t.Fatalf("expected dependency input DATABASE_URL, got %q", got)
	}
	if got := byID["auth/api"].RuntimeEnv["JWT_AUDIENCE"]; got != "payments" {
		t.Fatalf("expected profile env JWT_AUDIENCE, got %q", got)
	}
	if got := byID["auth/api"].RuntimeEnv["API_MODE"]; got != "strict" {
		t.Fatalf("expected service env API_MODE, got %q", got)
	}
	if got := byID["auth/stub"].RuntimeHeaders["x-suite-profile"]; got != "local.yaml" {
		t.Fatalf("expected x-suite-profile header for imported mock, got %q", got)
	}
	if got := byID["auth/api"].ResolvedRef; got != "localhost:5000/core/child-suite@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("expected resolved ref, got %q", got)
	}
}

type captureBackend struct {
	spec runner.StepSpec
}

func (b *captureBackend) ID() string                       { return "capture" }
func (b *captureBackend) Label() string                    { return "Capture" }
func (b *captureBackend) Kind() string                     { return "capture" }
func (b *captureBackend) IsAvailable(context.Context) bool { return true }
func (b *captureBackend) Run(_ context.Context, step runner.StepSpec, _ func(logstream.Line)) error {
	b.spec = step
	return nil
}

func TestRunNodePassesDependencyRuntimeToBackend(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	backend := &captureBackend{}
	suite := &suites.Definition{
		ID:         "parent-suite",
		Title:      "Parent Suite",
		Repository: "localhost:5000/core/parent-suite",
		Version:    "workspace",
		Topology: []suites.TopologyNode{
			{ID: "auth/api"},
		},
	}
	node := topologyNode{
		ID:               "auth/api",
		Name:             "auth/api",
		Kind:             "service",
		SourceSuiteID:    "child-suite",
		SourceSuiteTitle: "Child Suite",
		SourceRepository: "localhost:5000/core/child-suite",
		SourceVersion:    "workspace",
		DependencyAlias:  "auth-module",
		ResolvedRef:      "localhost:5000/core/child-suite@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Digest:           "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		RuntimeProfile:   "local.yaml",
		RuntimeEnv: map[string]string{
			"DATABASE_URL": "postgres://postgres:postgres@global-db:5432/auth",
		},
		RuntimeHeaders: map[string]string{
			"x-suite-profile": "local.yaml",
		},
	}

	if err := service.runNode(context.Background(), "run-runtime", suite, "parent.yaml", backend, node); err != nil {
		t.Fatalf("run node: %v", err)
	}

	if backend.spec.RuntimeProfile != "local.yaml" {
		t.Fatalf("expected runtime profile local.yaml, got %q", backend.spec.RuntimeProfile)
	}
	if got := backend.spec.Env["DATABASE_URL"]; got != "postgres://postgres:postgres@global-db:5432/auth" {
		t.Fatalf("expected runtime env DATABASE_URL, got %q", got)
	}
	if got := backend.spec.Headers["x-suite-profile"]; got != "local.yaml" {
		t.Fatalf("expected runtime header x-suite-profile, got %q", got)
	}
	if backend.spec.DependencyAlias != "auth-module" {
		t.Fatalf("expected dependency alias auth-module, got %q", backend.spec.DependencyAlias)
	}
	if backend.spec.SourceSuiteID != "child-suite" {
		t.Fatalf("expected source suite child-suite, got %q", backend.spec.SourceSuiteID)
	}
	if backend.spec.ResolvedRef != "localhost:5000/core/child-suite@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		t.Fatalf("expected resolved ref to round-trip, got %q", backend.spec.ResolvedRef)
	}
}

func TestRunNodePassesTrafficSpecToBackend(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	backend := &captureBackend{}
	suite := &suites.Definition{
		ID:    "traffic-suite",
		Title: "Traffic Suite",
		Topology: []suites.TopologyNode{
			{ID: "baseline"},
		},
	}
	node := topologyNode{
		ID:      "baseline",
		Name:    "baseline",
		Kind:    "traffic",
		Variant: "traffic.baseline",
		Load: &suites.LoadSpec{
			Variant:  "traffic.baseline",
			PlanPath: "traffic/smoke.star",
			Target:   "http://127.0.0.1:18080",
			Users: []suites.LoadUser{
				{
					Name: "probe",
					Tasks: []suites.LoadTask{
						{
							Name: "health",
							Request: suites.LoadRequest{
								Method: "GET",
								Path:   "/health",
								Name:   "health",
							},
						},
					},
				},
			},
			Stages: []suites.LoadStage{
				{Duration: time.Second, Users: 1, SpawnRate: 1},
			},
		},
	}

	if err := service.runNode(context.Background(), "run-traffic", suite, "local.yaml", backend, node); err != nil {
		t.Fatalf("run node: %v", err)
	}
	if backend.spec.Load == nil {
		t.Fatal("expected traffic spec to reach the backend")
	}
	if backend.spec.Load.Target != "http://127.0.0.1:18080" {
		t.Fatalf("expected traffic target to round-trip, got %q", backend.spec.Load.Target)
	}
	if backend.spec.Load.PlanPath != "traffic/smoke.star" {
		t.Fatalf("expected traffic plan path to round-trip, got %q", backend.spec.Load.PlanPath)
	}
}

func TestRunNodePassesEvaluationControlsToBackend(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	backend := &captureBackend{}
	suite := &suites.Definition{
		ID:    "evaluation-suite",
		Title: "Evaluation Suite",
		Topology: []suites.TopologyNode{
			{ID: "smoke"},
		},
	}
	expectExit := 0
	node := topologyNode{
		ID:      "smoke",
		Name:    "smoke",
		Kind:    "test",
		Variant: "test.run",
		Evaluation: &suites.StepEvaluation{
			ExpectExit: &expectExit,
			ExpectLogs: []string{"Test checks completed"},
			FailOnLogs: []string{"FATAL ERROR"},
		},
	}

	if err := service.runNode(context.Background(), "run-evaluation", suite, "local.yaml", backend, node); err != nil {
		t.Fatalf("run node: %v", err)
	}
	if backend.spec.Evaluation == nil || backend.spec.Evaluation.ExpectExit == nil || *backend.spec.Evaluation.ExpectExit != 0 {
		t.Fatalf("expected evaluation controls to reach backend, got %+v", backend.spec.Evaluation)
	}
	if len(backend.spec.Evaluation.ExpectLogs) != 1 || backend.spec.Evaluation.ExpectLogs[0] != "Test checks completed" {
		t.Fatalf("expected expect_logs to round-trip, got %+v", backend.spec.Evaluation)
	}
	if len(backend.spec.Evaluation.FailOnLogs) != 1 || backend.spec.Evaluation.FailOnLogs[0] != "FATAL ERROR" {
		t.Fatalf("expected fail_on_logs to round-trip, got %+v", backend.spec.Evaluation)
	}
}

func TestCreateExecutionMaterializesJUnitArtifactSummary(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"artifact-suite": {
				ID:    "artifact-suite",
				Title: "Artifact Suite",
				SuiteStar: strings.Join([]string{
					`smoke = test.run(file="smoke.py", image="python:3.12").export(path="reports/junit.xml", name="smoke-results", format="junit", on="always")`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "artifact-suite",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	waitForExecutionTerminal(t, service, execution.ID)

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(record.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(record.Artifacts))
	}
	if record.Artifacts[0].Format != "junit" {
		t.Fatalf("expected junit format, got %+v", record.Artifacts[0])
	}
	if record.Artifacts[0].TestSummary == nil {
		t.Fatalf("expected junit summary, got %+v", record.Artifacts[0])
	}
	if record.Artifacts[0].TestSummary.Total != 1 || record.Artifacts[0].TestSummary.Passed != 1 {
		t.Fatalf("expected passing junit summary, got %+v", record.Artifacts[0].TestSummary)
	}
}

func TestCreateExecutionContinueOnFailureAllowsDownstreamSteps(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"continue-suite": {
				ID:    "continue-suite",
				Title: "Continue Suite",
				SuiteStar: strings.Join([]string{
					`lint = test.run(file="lint.sh", image="bash:5.2", expect_logs="THIS STRING DOES NOT EXIST", continue_on_failure=true)`,
					`deploy = task.run(file="deploy.sh", image="bash:5.2", after=[lint])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "continue-suite",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	snapshot := waitForExecutionTerminal(t, service, execution.ID)
	if snapshot.Status != "Healthy" {
		t.Fatalf("expected healthy execution with continue_on_failure, got %q", snapshot.Status)
	}

	statuses := map[string]string{}
	for _, step := range snapshot.Steps {
		statuses[step.ID] = step.Status
	}
	if statuses["lint"] != "failed" {
		t.Fatalf("expected lint to fail softly, got %q", statuses["lint"])
	}
	if statuses["deploy"] != "healthy" {
		t.Fatalf("expected deploy to continue and succeed, got %q", statuses["deploy"])
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if !containsExecutionEventText(record.Events, "continue_on_failure is enabled") {
		t.Fatalf("expected continue_on_failure event in %+v", record.Events)
	}
}

func TestCreateExecutionRunsFailureHooksAndSkipsNormalDownstreamSteps(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"rollback-suite": {
				ID:    "rollback-suite",
				Title: "Rollback Suite",
				SuiteStar: strings.Join([]string{
					`primary = task.run(file="deploy.sh", image="bash:5.2", expect_logs="THIS STRING DOES NOT EXIST")`,
					`health = test.run(file="verify.sh", image="bash:5.2", after=[primary])`,
					`rollback = task.run(file="rollback.sh", image="bash:5.2", on_failure=[primary])`,
					`notify = task.run(file="notify.sh", image="bash:5.2", after=[rollback])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "rollback-suite",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	snapshot := waitForExecutionTerminal(t, service, execution.ID)
	if snapshot.Status != "Failed" {
		t.Fatalf("expected failed execution, got %q", snapshot.Status)
	}

	statuses := map[string]string{}
	for _, step := range snapshot.Steps {
		statuses[step.ID] = step.Status
	}
	if statuses["primary"] != "failed" {
		t.Fatalf("expected primary to fail, got %q", statuses["primary"])
	}
	if statuses["health"] != "skipped" {
		t.Fatalf("expected health to be skipped, got %q", statuses["health"])
	}
	if statuses["rollback"] != "healthy" {
		t.Fatalf("expected rollback to run, got %q", statuses["rollback"])
	}
	if statuses["notify"] != "healthy" {
		t.Fatalf("expected notify to run after rollback, got %q", statuses["notify"])
	}
	if snapshot.SkippedSteps != 1 {
		t.Fatalf("expected 1 skipped step, got %d", snapshot.SkippedSteps)
	}
}

func TestCreateExecutionResetsMockStateBeforeTestRun(t *testing.T) {
	source := staticSuiteSource{
		items: map[string]suites.Definition{
			"reset-suite": {
				ID:    "reset-suite",
				Title: "Reset Suite",
				SuiteStar: strings.Join([]string{
					`billing_mock = service.mock()`,
					`dirty_task = task.run(file="mess_up_state.sh", image="bash:5.2", after=[billing_mock])`,
					`clean_test = test.run(file="verify_billing.py", image="python:3.12", reset_mocks=[billing_mock], after=[dirty_task])`,
				}, "\n"),
				Profiles: []suites.ProfileOption{
					{FileName: "local.yaml", Default: true},
				},
			},
		},
	}

	service := NewService(source)
	resetter := &stubMockResetter{}
	service.ConfigureMockResetter(resetter)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "reset-suite",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	snapshot := waitForExecutionTerminal(t, service, execution.ID)
	if snapshot.Status != "Healthy" {
		t.Fatalf("expected healthy execution, got %q", snapshot.Status)
	}
	if len(resetter.suiteIDs) != 1 || resetter.suiteIDs[0] != "reset-suite" {
		t.Fatalf("expected mock reset for reset-suite, got %+v", resetter.suiteIDs)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if !containsExecutionEventText(record.Events, "Resetting mock state") {
		t.Fatalf("expected reset_mocks event in %+v", record.Events)
	}
}

func TestCreateExecutionUsesPersistedSuiteProfileSet(t *testing.T) {
	profileService := profiles.NewService(suites.NewService(), profiles.NewMemoryStore())
	if _, err := profileService.CreateProfile("payment-suite", profiles.UpsertRequest{
		Name:        "Holiday Freeze",
		FileName:    "holiday.yaml",
		Description: "Freeze routing for end-of-quarter reconciliation.",
		Scope:       "Staging",
		YAML:        "env:\n  LEDGER_PERIOD: holiday\nservices:\n  workerReplicaCount: 2\n",
		ExtendsID:   "base",
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	service := NewService(profileService)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "holiday.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if execution.Profile != "holiday.yaml" {
		t.Fatalf("expected holiday.yaml profile, got %q", execution.Profile)
	}
}

func containsTopologyDependency(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsExecutionEventText(events []ExecutionEvent, needle string) bool {
	for _, event := range events {
		if strings.Contains(event.Text, needle) {
			return true
		}
	}
	return false
}

func waitForExecutionTerminal(t *testing.T, service *Service, executionID string) Snapshot {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, ok := service.snapshotExecution(executionID)
		if ok && snapshot.RunningSteps == 0 && snapshot.PendingSteps == 0 && snapshot.Status != "Booting" {
			return snapshot
		}
		time.Sleep(40 * time.Millisecond)
	}

	snapshot, _ := service.snapshotExecution(executionID)
	t.Fatalf("timed out waiting for execution %s to finish; last snapshot: %+v", executionID, snapshot)
	return Snapshot{}
}

func TestCreateExecutionSyncsObservers(t *testing.T) {
	observer := &captureObserver{events: make(chan Snapshot, 8)}
	service := NewService(suites.NewService(), observer)
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case snapshot := <-observer.events:
			if snapshot.ID != execution.ID {
				continue
			}
			if snapshot.Profile != "year.yaml" {
				t.Fatalf("expected year.yaml profile, got %q", snapshot.Profile)
			}
			if snapshot.TotalSteps == 0 {
				t.Fatal("expected execution snapshot to include topology steps")
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for observer sync")
		}
	}
}

func TestSubscribeEventsReplaysAndStreams(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	stream, err := service.SubscribeEvents(context.Background(), "run-1043", 0)
	if err != nil {
		t.Fatalf("subscribe existing execution: %v", err)
	}

	select {
	case event := <-stream:
		if event.ID != 1 {
			t.Fatalf("expected first replay event id 1, got %d", event.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay event")
	}

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	liveStream, err := service.SubscribeEvents(context.Background(), execution.ID, 0)
	if err != nil {
		t.Fatalf("subscribe live execution: %v", err)
	}

	select {
	case event := <-liveStream:
		if event.Event.Source == "" {
			t.Fatal("expected streamed event source")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live event")
	}
}

func TestSubscribeLogsReplaysAndStreams(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	stream, err := service.SubscribeLogs(context.Background(), "run-1043", 0)
	if err != nil {
		t.Fatalf("subscribe existing execution logs: %v", err)
	}

	select {
	case line := <-stream:
		if line.ID != 1 {
			t.Fatalf("expected first replay log id 1, got %d", line.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay log line")
	}

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "payment-suite",
		Profile: "year.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	liveStream, err := service.SubscribeLogs(context.Background(), execution.ID, 0)
	if err != nil {
		t.Fatalf("subscribe live execution logs: %v", err)
	}

	select {
	case line := <-liveStream:
		if line.Line.Source == "" {
			t.Fatal("expected streamed log line source")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live log line")
	}
}

func TestStorefrontExecutionUsesScenarioOnlyTopology(t *testing.T) {
	observer := &captureObserver{events: make(chan Snapshot, 16)}
	service := NewService(suites.NewService(), observer)
	defer service.Close()

	_, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "storefront-browser-lab",
		Profile: "promo.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case snapshot := <-observer.events:
			foundTest := false
			for _, step := range snapshot.Steps {
				if step.Kind == "traffic" {
					t.Fatal("did not expect storefront execution to include a traffic step")
				}
				if step.Kind == "test" && step.ID == "playwright_checkout" {
					foundTest = true
				}
			}
			if foundTest {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for storefront test step to appear in execution snapshot")
		}
	}
}

func TestGetExecutionIncludesSuiteSourceFiles(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "returns-control-plane",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(record.Suite.Folders) == 0 {
		t.Fatal("expected execution suite folders")
	}
	if len(record.Suite.SourceFiles) == 0 {
		t.Fatal("expected execution suite source files")
	}
	if len(record.Suite.APISurfaces) == 0 {
		t.Fatal("expected execution suite api surfaces")
	}

	foundMockFile := false
	for _, file := range record.Suite.SourceFiles {
		if file.Path != "mock/returns/create-return.cue" {
			continue
		}
		foundMockFile = true
		if !strings.Contains(file.Content, `@gen(`) {
			t.Fatalf("expected declarative mock generation content, got %q", file.Content)
		}
	}

	if !foundMockFile {
		t.Fatal("expected returns execution to include mock source file")
	}
}

func TestGetExecutionRendersGeneratedMockPreviewData(t *testing.T) {
	service := NewService(suites.NewService())
	defer service.Close()

	execution, err := service.CreateExecution(context.Background(), CreateRequest{
		SuiteID: "returns-control-plane",
		Profile: "local.yaml",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}

	record, err := service.GetExecution(execution.ID)
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}

	var preview string
	for _, surface := range record.Suite.APISurfaces {
		if surface.ID != "returns-api" {
			continue
		}
		for _, operation := range surface.Operations {
			if operation.ID != "create-return" {
				continue
			}
			for _, exchange := range operation.Exchanges {
				if exchange.Name == "approved-standard" {
					preview = exchange.ResponseBody
				}
			}
		}
	}

	if strings.TrimSpace(preview) == "" {
		t.Fatal("expected create-return preview body")
	}
	if strings.Contains(preview, "{{") {
		t.Fatalf("expected rendered preview body, got %q", preview)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(preview), &payload); err != nil {
		t.Fatalf("preview body should be valid json: %v", err)
	}

	returnID, _ := payload["returnId"].(string)
	traceID, _ := payload["traceId"].(string)
	servedAt, _ := payload["servedAt"].(string)
	returnID = strings.TrimSpace(returnID)
	traceID = strings.TrimSpace(traceID)
	servedAt = strings.TrimSpace(servedAt)
	if !strings.HasPrefix(returnID, "ret_") {
		t.Fatalf("expected generated returnId, got %q", returnID)
	}
	if traceID == "" {
		t.Fatal("expected generated traceId")
	}
	if servedAt == "" {
		t.Fatal("expected generated servedAt")
	}
}
