package execution

import (
	"errors"
	"testing"

	"github.com/babelsuite/babelsuite/internal/suites"
)

func TestParseSuiteTopologySupportsUnifiedRuntimeEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service", "task", "test")

cache = service.run(image="redis:alpine", ports={"6379": 6379})
seed = task.run(image="bash:5.2", file="seed.sh", after=[cache])
worker = service.run(image="ghcr.io/acme/worker:latest", env={"REDIS_ADDR": "cache:6379"}, after=[cache, seed])
shared = service.run(name_or_id="otel-collector", after=[worker])
smoke = test.run(image="golang:1.24", file="go/smoke_test.go", after=[shared])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 5 {
		t.Fatalf("expected 5 topology nodes, got %d", len(topology))
	}

	assertNode := func(index int, expectedID string, expectedLevel int, expectedDeps ...string) {
		t.Helper()
		node := topology[index]
		if node.Kind != "service" && expectedID != "seed" && expectedID != "smoke" {
			t.Fatalf("expected service kind for %s, got %s", expectedID, node.Kind)
		}
		if node.ID != expectedID {
			t.Fatalf("expected node id %q at index %d, got %q", expectedID, index, node.ID)
		}
		if node.Level != expectedLevel {
			t.Fatalf("expected node %q level %d, got %d", expectedID, expectedLevel, node.Level)
		}
		if len(node.DependsOn) != len(expectedDeps) {
			t.Fatalf("expected node %q deps %v, got %v", expectedID, expectedDeps, node.DependsOn)
		}
		for dependencyIndex, dependency := range expectedDeps {
			if node.DependsOn[dependencyIndex] != dependency {
				t.Fatalf("expected node %q deps %v, got %v", expectedID, expectedDeps, node.DependsOn)
			}
		}
	}

	assertNode(0, "cache", 0)
	if topology[1].Kind != "task" {
		t.Fatalf("expected task kind for seed, got %s", topology[1].Kind)
	}
	assertNode(1, "seed", 1, "cache")
	assertNode(2, "worker", 2, "cache", "seed")
	assertNode(3, "otel-collector", 3, "worker")
	if topology[4].Kind != "test" {
		t.Fatalf("expected test kind for smoke, got %s", topology[4].Kind)
	}
	assertNode(4, "smoke", 4, "otel-collector")
}

func TestParseSuiteTopologyDefaultsNodeIDToAssignmentAndSupportsIdentifierDependencies(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service", "task", "test")

db = service.run()
seed_data = task.run(file="seed.sh", image="bash:5.2", after=[db])
smoke_test = test.run(file="smoke_test.py", image="python:3.12", after=[seed_data])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 topology nodes, got %d", len(topology))
	}
	if topology[0].ID != "db" || topology[1].ID != "seed_data" || topology[2].ID != "smoke_test" {
		t.Fatalf("expected assignment names to become ids, got %q, %q, %q", topology[0].ID, topology[1].ID, topology[2].ID)
	}
	if len(topology[1].DependsOn) != 1 || topology[1].DependsOn[0] != "db" {
		t.Fatalf("expected seed_data to depend on db, got %v", topology[1].DependsOn)
	}
	if len(topology[2].DependsOn) != 1 || topology[2].DependsOn[0] != "seed_data" {
		t.Fatalf("expected smoke_test to depend on seed_data, got %v", topology[2].DependsOn)
	}
}

func TestParseSuiteTopologySupportsArtifactExportChains(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "test")

smoke = test.run(file="smoke_test.py", image="python:3.12") \
  .export("coverage/*.xml", name="go-coverage", on="always", format="cobertura") \
  .export("logs/crash.dump", name="crash-debug", on="failure")`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 1 {
		t.Fatalf("expected 1 topology node, got %d", len(topology))
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

func TestParseSuiteTopologySupportsEvaluationControls(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "task", "test")

seed = task.run(file="seed.sh", image="bash:5.2", expect_exit=0, expect_logs=["Task completed successfully"], fail_on_logs="FATAL ERROR")
smoke = test.run(file="smoke_test.py", image="python:3.12", after=[seed], expect_logs="Test checks completed")`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 2 {
		t.Fatalf("expected 2 topology nodes, got %d", len(topology))
	}
	if topology[0].Evaluation == nil || topology[0].Evaluation.ExpectExit == nil || *topology[0].Evaluation.ExpectExit != 0 {
		t.Fatalf("expected seed expect_exit=0, got %+v", topology[0].Evaluation)
	}
	if len(topology[0].Evaluation.ExpectLogs) != 1 || topology[0].Evaluation.ExpectLogs[0] != "Task completed successfully" {
		t.Fatalf("expected seed expect_logs, got %+v", topology[0].Evaluation)
	}
	if len(topology[0].Evaluation.FailOnLogs) != 1 || topology[0].Evaluation.FailOnLogs[0] != "FATAL ERROR" {
		t.Fatalf("expected seed fail_on_logs, got %+v", topology[0].Evaluation)
	}
	if topology[1].Evaluation == nil || len(topology[1].Evaluation.ExpectLogs) != 1 || topology[1].Evaluation.ExpectLogs[0] != "Test checks completed" {
		t.Fatalf("expected smoke expect_logs, got %+v", topology[1].Evaluation)
	}
}

func TestParseSuiteTopologySupportsFailureFlowControls(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service", "task", "test")

billing_mock = service.mock()
lint = test.run(file="lint.sh", image="bash:5.2", expect_logs="THIS STRING DOES NOT EXIST", continue_on_failure=true, reset_mocks=[billing_mock])
rollback = task.run(file="rollback.sh", image="bash:5.2", on_failure=[lint])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 topology nodes, got %d", len(topology))
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
		t.Fatalf("expected rollback to depend on lint terminal state, got %+v", topology[2].DependsOn)
	}
}

func TestParseSuiteTopologyRejectsLegacyCompatibilityAliases(t *testing.T) {
	tests := []struct {
		name      string
		suiteStar string
		want      string
	}{
		{
			name: "container",
			suiteStar: `load("@babelsuite/runtime", "container")

shared = container.get(id="otel-collector")`,
			want: "unsupported runtime call \"container.get\"",
		},
		{
			name: "script",
			suiteStar: `load("@babelsuite/runtime", "script")

seed = script.file(file="seed.sh", image="bash:5.2")`,
			want: "unsupported runtime call \"script.file\"",
		},
		{
			name: "scenario",
			suiteStar: `load("@babelsuite/runtime", "scenario")

smoke = scenario.python(file="smoke_test.py", image="python:3.12")`,
			want: "unsupported runtime call \"scenario.python\"",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSuiteTopology(test.suiteStar)
			if err == nil {
				t.Fatal("expected invalid topology error")
			}
			if !errors.Is(err, ErrInvalidTopology) {
				t.Fatalf("expected invalid topology error, got %v", err)
			}
			if !errors.Is(err, suites.ErrUnsupportedCall) {
				t.Fatalf("expected unsupported call error, got %v", err)
			}
		})
	}
}

func TestParseSuiteTopologySupportsMockAndServiceModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service", "test")

payments_api = service.run(name="payments-api")
orders = service.mock(name="orders-mock", source="mock/orders", after=["payments-api"])
catalog = service.prism(name="catalog-compat", spec_path="./compat/prism/openapi.yaml", port=4010, after=["orders-mock"])
legacy = service.custom(name="legacy-compat", command=["node", "./services/custom_mock.js"], port=9090, after=["catalog-compat"])
smoke = test.run(name="smoke", file="http/smoke.hurl", image="curlimages/curl:8.7.1", after=["legacy-compat"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 5 {
		t.Fatalf("expected 5 topology nodes, got %d", len(topology))
	}

	expected := []struct {
		id   string
		kind string
		deps []string
	}{
		{id: "payments-api", kind: "service"},
		{id: "orders-mock", kind: "mock", deps: []string{"payments-api"}},
		{id: "catalog-compat", kind: "service", deps: []string{"orders-mock"}},
		{id: "legacy-compat", kind: "service", deps: []string{"catalog-compat"}},
		{id: "smoke", kind: "test", deps: []string{"legacy-compat"}},
	}

	for index, item := range expected {
		node := topology[index]
		if node.ID != item.id {
			t.Fatalf("expected node %d id %q, got %q", index, item.id, node.ID)
		}
		if node.Kind != item.kind {
			t.Fatalf("expected node %q kind %q, got %q", item.id, item.kind, node.Kind)
		}
		if len(node.DependsOn) != len(item.deps) {
			t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
		}
		for dependencyIndex, dependency := range item.deps {
			if node.DependsOn[dependencyIndex] != dependency {
				t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
			}
		}
	}
}

func TestParseSuiteTopologySupportsTaskModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "task", "test")

bootstrap = task.run(name="bootstrap", file="bootstrap.sh", image="bash:5.2")
migrate = task.run(name="migrate-db", file="migrate.sql", image="postgres:16", after=["bootstrap"])
seed = task.run(name="seed-data", file="seed.ts", image="node:22", after=["migrate-db"])
smoke = test.run(name="smoke", file="go/smoke_test.go", image="golang:1.24", after=["seed-data"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 4 {
		t.Fatalf("expected 4 topology nodes, got %d", len(topology))
	}

	expected := []struct {
		id   string
		kind string
		deps []string
	}{
		{id: "bootstrap", kind: "task"},
		{id: "migrate-db", kind: "task", deps: []string{"bootstrap"}},
		{id: "seed-data", kind: "task", deps: []string{"migrate-db"}},
		{id: "smoke", kind: "test", deps: []string{"seed-data"}},
	}

	for index, item := range expected {
		node := topology[index]
		if node.ID != item.id {
			t.Fatalf("expected node %d id %q, got %q", index, item.id, node.ID)
		}
		if node.Kind != item.kind {
			t.Fatalf("expected node %q kind %q, got %q", item.id, item.kind, node.Kind)
		}
		if len(node.DependsOn) != len(item.deps) {
			t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
		}
		for dependencyIndex, dependency := range item.deps {
			if node.DependsOn[dependencyIndex] != dependency {
				t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
			}
		}
	}
}

func TestParseSuiteTopologySupportsTestModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "test")

go_smoke = test.run(name="checkout-go", file="go/checkout_test.go", image="golang:1.24")
python_smoke = test.run(name="checkout-python", file="checkout.py", image="python:3.12", after=["checkout-go"])
http_smoke = test.run(name="checkout-http", file="http/checkout.hurl", image="curlimages/curl:8.7.1", after=["checkout-python"])
playwright_smoke = test.run(name="checkout-ui", file="playwright/checkout.spec.ts", image="mcr.microsoft.com/playwright:v1.53.0-noble", after=["checkout-http"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 4 {
		t.Fatalf("expected 4 topology nodes, got %d", len(topology))
	}

	expected := []struct {
		id   string
		deps []string
	}{
		{id: "checkout-go"},
		{id: "checkout-python", deps: []string{"checkout-go"}},
		{id: "checkout-http", deps: []string{"checkout-python"}},
		{id: "checkout-ui", deps: []string{"checkout-http"}},
	}

	for index, item := range expected {
		node := topology[index]
		if node.ID != item.id {
			t.Fatalf("expected node %d id %q, got %q", index, item.id, node.ID)
		}
		if node.Kind != "test" {
			t.Fatalf("expected node %q kind test, got %q", item.id, node.Kind)
		}
		if len(node.DependsOn) != len(item.deps) {
			t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
		}
		for dependencyIndex, dependency := range item.deps {
			if node.DependsOn[dependencyIndex] != dependency {
				t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
			}
		}
	}
}

func TestParseSuiteTopologySupportsTrafficModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "traffic")

	smoke = traffic.smoke(name="smoke-traffic", plan="smoke.star", target="http://payments-api:8080")
	baseline = traffic.baseline(name="baseline-traffic", plan="baseline.star", target="http://payments-api:8080", after=["smoke-traffic"])
	stress = traffic.stress(name="stress-traffic", plan="stress.star", target="http://payments-api:8080", after=["baseline-traffic"])
	spike = traffic.spike(name="spike-traffic", plan="spike.star", target="http://payments-api:8080", after=["stress-traffic"])
	soak = traffic.soak(name="soak-traffic", plan="soak.star", target="http://payments-api:8080", after=["spike-traffic"])
	scalability = traffic.scalability(name="scalability-traffic", plan="scalability.star", target="http://payments-api:8080", after=["soak-traffic"])
	step = traffic.step(name="step-traffic", plan="step.star", target="http://payments-api:8080", after=["scalability-traffic"])
	wave = traffic.wave(name="wave-traffic", plan="wave.star", target="http://payments-api:8080", after=["step-traffic"])
	staged = traffic.staged(name="staged-traffic", plan="staged.star", target="http://payments-api:8080", after=["wave-traffic"])
	throughput = traffic.constant_throughput(name="constant-throughput-traffic", plan="throughput.star", target="http://payments-api:8080", after=["staged-traffic"])
	pacing = traffic.constant_pacing(name="constant-pacing-traffic", plan="pacing.star", target="http://payments-api:8080", after=["constant-throughput-traffic"])
	arrival = traffic.open_model(name="open-model-traffic", plan="open_model.star", target="http://payments-api:8080", after=["constant-pacing-traffic"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 12 {
		t.Fatalf("expected 12 topology nodes, got %d", len(topology))
	}

	expected := []struct {
		id      string
		variant string
		deps    []string
	}{
		{id: "smoke-traffic", variant: "traffic.smoke"},
		{id: "baseline-traffic", variant: "traffic.baseline", deps: []string{"smoke-traffic"}},
		{id: "stress-traffic", variant: "traffic.stress", deps: []string{"baseline-traffic"}},
		{id: "spike-traffic", variant: "traffic.spike", deps: []string{"stress-traffic"}},
		{id: "soak-traffic", variant: "traffic.soak", deps: []string{"spike-traffic"}},
		{id: "scalability-traffic", variant: "traffic.scalability", deps: []string{"soak-traffic"}},
		{id: "step-traffic", variant: "traffic.step", deps: []string{"scalability-traffic"}},
		{id: "wave-traffic", variant: "traffic.wave", deps: []string{"step-traffic"}},
		{id: "staged-traffic", variant: "traffic.staged", deps: []string{"wave-traffic"}},
		{id: "constant-throughput-traffic", variant: "traffic.constant_throughput", deps: []string{"staged-traffic"}},
		{id: "constant-pacing-traffic", variant: "traffic.constant_pacing", deps: []string{"constant-throughput-traffic"}},
		{id: "open-model-traffic", variant: "traffic.open_model", deps: []string{"constant-pacing-traffic"}},
	}

	for index, item := range expected {
		node := topology[index]
		if node.ID != item.id {
			t.Fatalf("expected node %d id %q, got %q", index, item.id, node.ID)
		}
		if node.Kind != "traffic" {
			t.Fatalf("expected node %q kind traffic, got %q", item.id, node.Kind)
		}
		if node.Variant != item.variant {
			t.Fatalf("expected node %q variant %q, got %q", item.id, item.variant, node.Variant)
		}
		if len(node.DependsOn) != len(item.deps) {
			t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
		}
		for dependencyIndex, dependency := range item.deps {
			if node.DependsOn[dependencyIndex] != dependency {
				t.Fatalf("expected node %q deps %v, got %v", item.id, item.deps, node.DependsOn)
			}
		}
	}
}

func TestParseSuiteTopologyRejectsMissingDependency(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service")

api = service.run(name="api", after=["db"])`

	_, err := parseSuiteTopology(suiteStar)
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected invalid topology error, got %v", err)
	}
	if !errors.Is(err, suites.ErrMissingDependency) {
		t.Fatalf("expected missing dependency error, got %v", err)
	}
}

func TestParseSuiteTopologyRejectsCycles(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service")

api = service.run(name="api", after=["db"])
db = service.run(name="db", after=["api"])`

	_, err := parseSuiteTopology(suiteStar)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected invalid topology error, got %v", err)
	}
	if !errors.Is(err, suites.ErrTopologyCycle) {
		t.Fatalf("expected topology cycle error, got %v", err)
	}
}

func TestParseSuiteTopologyUsesStableDeclarationOrderWithinLevels(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "service")

zeta = service.run(name="zeta")
alpha = service.run(name="alpha")
worker = service.run(name="worker", after=["zeta", "alpha"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 topology nodes, got %d", len(topology))
	}
	if topology[0].ID != "zeta" || topology[1].ID != "alpha" {
		t.Fatalf("expected declaration order within the first level, got %q then %q", topology[0].ID, topology[1].ID)
	}
	if topology[0].Level != 0 || topology[1].Level != 0 || topology[2].Level != 1 {
		t.Fatalf("expected layered topology levels, got %d, %d, %d", topology[0].Level, topology[1].Level, topology[2].Level)
	}
}
