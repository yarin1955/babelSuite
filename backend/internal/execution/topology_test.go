package execution

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSuiteTopologySupportsContainerModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "container", "mock", "script", "scenario")

cache = container.run(name="cache", image="redis:alpine", ports={"6379": 6379})
seed = script(name="seed", after=["cache"])
worker = container.create(name="worker", image="ghcr.io/acme/worker:latest", env={"REDIS_ADDR": "cache:6379"}, after=["cache", "seed"])
	shared = container.get(name_or_id="otel-collector", after=["worker"])
	smoke = scenario(name="smoke", after=["otel-collector"])`

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
		if node.Kind != "container" && expectedID != "seed" && expectedID != "smoke" {
			t.Fatalf("expected container kind for %s, got %s", expectedID, node.Kind)
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
	if topology[1].Kind != "script" {
		t.Fatalf("expected script kind for seed, got %s", topology[1].Kind)
	}
	assertNode(1, "seed", 1, "cache")
	assertNode(2, "worker", 2, "cache", "seed")
	assertNode(3, "otel-collector", 3, "worker")
	if topology[4].Kind != "scenario" {
		t.Fatalf("expected scenario kind for smoke, got %s", topology[4].Kind)
	}
	assertNode(4, "smoke", 4, "otel-collector")
}

func TestParseSuiteTopologySupportsContainerGetByID(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "container")

shared = container.get(id="otel-collector")`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 1 {
		t.Fatalf("expected one topology node, got %d", len(topology))
	}
	if topology[0].ID != "otel-collector" {
		t.Fatalf("expected id-based get to use otel-collector, got %q", topology[0].ID)
	}
	if topology[0].Kind != "container" {
		t.Fatalf("expected kind container, got %q", topology[0].Kind)
	}
}

func TestParseSuiteTopologySupportsMockAndServiceModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "container", "mock", "service", "scenario")

payments_api = container.run(name="payments-api")
orders = mock.serve(name="orders-mock", source="./mock/orders", after=["payments-api"])
catalog = service.prism(name="catalog-compat", spec_path="./compat/prism/openapi.yaml", port=4010, after=["orders-mock"])
legacy = service.custom(name="legacy-compat", command=["node", "./scripts/custom_mock.js"], port=9090, after=["catalog-compat"])
smoke = scenario(name="smoke", after=["legacy-compat"])`

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
		{id: "payments-api", kind: "container"},
		{id: "orders-mock", kind: "mock", deps: []string{"payments-api"}},
		{id: "catalog-compat", kind: "service", deps: []string{"orders-mock"}},
		{id: "legacy-compat", kind: "service", deps: []string{"catalog-compat"}},
		{id: "smoke", kind: "scenario", deps: []string{"legacy-compat"}},
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

func TestParseSuiteTopologySupportsScriptModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "script", "scenario")

bootstrap = script.file(name="bootstrap", file_path="./scripts/bootstrap.sh", interpreter="bash")
migrate = script.sql_migrate(name="migrate-db", target="db", sql_dir="./sql", after=["bootstrap"])
seed = script.exec(name="seed-data", command=["make", "seed"], cwd="./scripts", after=["migrate-db"])
smoke = scenario(name="smoke", after=["seed-data"])`

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
		{id: "bootstrap", kind: "script"},
		{id: "migrate-db", kind: "script", deps: []string{"bootstrap"}},
		{id: "seed-data", kind: "script", deps: []string{"migrate-db"}},
		{id: "smoke", kind: "scenario", deps: []string{"seed-data"}},
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

func TestParseSuiteTopologySupportsScenarioModuleEntrypoints(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "scenario")

go_smoke = scenario.go(name="checkout-go", test_dir="./scenarios/go", objectives=["checkout", "payments"], tags=["smoke", "ci"])
python_smoke = scenario.python(name="checkout-python", test_dir="./scenarios/python", objectives=["checkout"], tags=["regression"], after=["checkout-go"])
http_smoke = scenario.http(name="checkout-http", collection_path="./scenarios/http/checkout.hurl", objectives=["edge"], tags=["api"], after=["checkout-python"])`

	topology, err := parseSuiteTopology(suiteStar)
	if err != nil {
		t.Fatalf("parse topology: %v", err)
	}
	if len(topology) != 3 {
		t.Fatalf("expected 3 topology nodes, got %d", len(topology))
	}

	expected := []struct {
		id   string
		deps []string
	}{
		{id: "checkout-go"},
		{id: "checkout-python", deps: []string{"checkout-go"}},
		{id: "checkout-http", deps: []string{"checkout-python"}},
	}

	for index, item := range expected {
		node := topology[index]
		if node.ID != item.id {
			t.Fatalf("expected node %d id %q, got %q", index, item.id, node.ID)
		}
		if node.Kind != "scenario" {
			t.Fatalf("expected node %q kind scenario, got %q", item.id, node.Kind)
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
	suiteStar := `load("@babelsuite/runtime", "container")

api = container.run(name="api", after=["db"])`

	_, err := parseSuiteTopology(suiteStar)
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected invalid topology error, got %v", err)
	}
	if !strings.Contains(err.Error(), `depends on missing step "db"`) {
		t.Fatalf("expected missing dependency details, got %v", err)
	}
}

func TestParseSuiteTopologyRejectsCycles(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "container")

api = container.run(name="api", after=["db"])
db = container.run(name="db", after=["api"])`

	_, err := parseSuiteTopology(suiteStar)
	if err == nil {
		t.Fatal("expected cycle error")
	}
	if !errors.Is(err, ErrInvalidTopology) {
		t.Fatalf("expected invalid topology error, got %v", err)
	}
	if !strings.Contains(err.Error(), "dependency cycle detected") {
		t.Fatalf("expected cycle details, got %v", err)
	}
}

func TestParseSuiteTopologyUsesStableDeclarationOrderWithinLevels(t *testing.T) {
	suiteStar := `load("@babelsuite/runtime", "container")

zeta = container.run(name="zeta")
alpha = container.run(name="alpha")
worker = container.run(name="worker", after=["zeta", "alpha"])`

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
