package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/examplefs"
	"github.com/babelsuite/babelsuite/internal/examplegen"
	"github.com/babelsuite/babelsuite/internal/suites"
)

type RenderedFile struct {
	Path    string
	Content string
}

func RenderWorkspaceFiles() []RenderedFile {
	files := make([]RenderedFile, 0)

	service := suites.NewService()
	for _, suite := range service.List() {
		base := joinPath("oci-suites", suite.ID)
		files = append(files,
			RenderedFile{Path: joinPath(base, "README.md"), Content: renderSuiteReadme(suite)},
			RenderedFile{Path: joinPath(base, "suite.star"), Content: ensureTrailingNewline(suite.SuiteStar)},
		)
		for _, file := range examplegen.GeneratedSourceFiles(suite) {
			if !shouldWriteExampleSourceFile(file.Path) {
				continue
			}
			files = append(files, RenderedFile{
				Path:    joinPath(base, file.Path),
				Content: ensureTrailingNewline(file.Content),
			})
		}
	}

	for _, module := range catalog.SeedStdlibPackages() {
		base := joinPath("oci-modules", moduleDirectoryName(module))
		files = append(files,
			RenderedFile{Path: joinPath(base, "README.md"), Content: renderModuleReadme(module)},
			RenderedFile{Path: joinPath(base, "module.yaml"), Content: renderModuleMetadata(module)},
			RenderedFile{Path: joinPath(base, "usage.star"), Content: renderModuleUsage(module)},
		)
		for _, extra := range renderModuleExtraFiles(module) {
			files = append(files, RenderedFile{
				Path:    joinPath(base, extra.Path),
				Content: ensureTrailingNewline(extra.Content),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func SyncWorkspace(repoRoot string) (int, error) {
	files := RenderWorkspaceFiles()
	examplesRoot := examplefs.ResolveRootFromRepo(repoRoot)
	if err := cleanupGeneratedSuiteArtifacts(examplesRoot); err != nil {
		return 0, err
	}
	for _, file := range files {
		target := filepath.Join(examplesRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

func renderSuiteReadme(suite suites.Definition) string {
	lines := []string{
		suite.Title,
		"",
		suite.Description,
		"",
		"Structure",
		"",
		"- `suite.star`: declarative topology",
	}
	for _, folder := range suite.Folders {
		if !shouldDescribeExampleFolder(folder.Name) {
			continue
		}
		lines = append(lines, fmt.Sprintf("- `%s/`: %s", folder.Name, folder.Description))
	}
	return strings.Join(lines, "\n") + "\n"
}

func shouldWriteExampleSourceFile(path string) bool {
	normalized := filepath.ToSlash(strings.Trim(strings.TrimSpace(path), "/"))
	return !strings.HasPrefix(normalized, "gateway/")
}

func shouldDescribeExampleFolder(name string) bool {
	return strings.TrimSpace(name) != "gateway"
}

func cleanupGeneratedSuiteArtifacts(examplesRoot string) error {
	matches, err := filepath.Glob(filepath.Join(examplesRoot, "oci-suites", "*", "gateway"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.RemoveAll(match); err != nil {
			return err
		}
	}

	suitesRoot := filepath.Join(examplesRoot, "oci-suites")
	if err := filepath.WalkDir(suitesRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		segments := strings.Split(filepath.ToSlash(path), "/")
		for _, segment := range segments {
			if segment != "mock" {
				continue
			}
			return os.Remove(path)
		}
		return nil
	}); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renderModuleReadme(module catalog.Package) string {
	usageLine := "See `usage.star` for a minimal Starlark import example."
	structure := []string{}
	if moduleDirectoryName(module) == "runtime" {
		usageLine = "See `usage.star`, `suite_example.star`, `scripts/container_lifecycle.star`, `scripts/mock_lifecycle.star`, `scripts/service_lifecycle.star`, `scripts/script_results.star`, and `scenario_reports.star` for the recommended runtime patterns."
		structure = []string{
			"",
			"Runtime Examples",
			"",
			"- `usage.star`: quick overview of `container.*`, native `mock.*`, external `service.*`, `script.*`, and `scenario.*` entry points plus object methods.",
			"- `suite_example.star`: declarative topology that keeps `suite.star` easy to read.",
			"- `scripts/container_lifecycle.star`: imperative container operations such as `exec`, `copy`, `logs`, `inspect`, and teardown.",
			"- `scripts/mock_lifecycle.star`: native mock operations such as `wait_ready`, `url`, `logs`, `reset_state`, and `preview`.",
			"- `scripts/service_lifecycle.star`: external compatibility service operations such as `wait_ready`, `url`, `logs`, `stop`, and `kill`.",
			"- `scripts/script_results.star`: synchronous script execution with `exit_code`, `stdout`, `stderr`, and `assert_success()`.",
			"- `scenario_reports.star`: Go, Python, and HTTP scenario execution with `passed`, `exit_code`, `duration_ms`, `logs`, `summary`, and `artifacts_dir`.",
			"- `mock/`: sample BabelSuite-native mock assets.",
			"- `compat/`: sample WireMock and Prism compatibility assets.",
			"- `scripts/` and `sql/`: sample assets for `script.file`, the `script.bash` convenience sugar, and `script.sql_migrate`.",
			"- `scenarios/`: sample Go, pytest, and Hurl-style scenario assets.",
		}
	}

	lines := []string{
		module.Title,
		"",
		module.Description,
		"",
		"Details",
		"",
		fmt.Sprintf("- Repository: `%s`", module.Repository),
		fmt.Sprintf("- Version: `%s`", module.Version),
		fmt.Sprintf("- Tags: `%s`", strings.Join(module.Tags, "`, `")),
		fmt.Sprintf("- Pull: `%s`", module.PullCommand),
		fmt.Sprintf("- Fork: `%s`", module.ForkCommand),
		"",
		"Usage",
		"",
		usageLine,
	}
	lines = append(lines, structure...)
	return strings.Join(lines, "\n") + "\n"
}

func renderModuleMetadata(module catalog.Package) string {
	lines := []string{
		"kind: OCIExampleModule",
		"metadata:",
		fmt.Sprintf("  id: %s", module.ID),
		fmt.Sprintf("  title: %s", module.Title),
		"spec:",
		fmt.Sprintf("  repository: %s", module.Repository),
		fmt.Sprintf("  provider: %s", module.Provider),
		fmt.Sprintf("  version: %s", module.Version),
		"  tags:",
	}
	for _, tag := range module.Tags {
		lines = append(lines, fmt.Sprintf("    - %s", tag))
	}
	lines = append(lines,
		fmt.Sprintf("  description: %s", module.Description),
		fmt.Sprintf("  pullCommand: %s", module.PullCommand),
		fmt.Sprintf("  forkCommand: %s", module.ForkCommand),
	)
	return strings.Join(lines, "\n") + "\n"
}

func renderModuleUsage(module catalog.Package) string {
	if moduleDirectoryName(module) == "runtime" {
		lines := []string{
			`load("@babelsuite/runtime", "container", "mock", "service", "script", "scenario")`,
			"",
			"# Container entry points with after= preserved.",
			`cache = container.run(`,
			`    name="redis-cache",`,
			`    image="redis:alpine",`,
			`    after=["migrate-db"],`,
			`    env={"ALLOW_EMPTY_PASSWORD": "yes"},`,
			`    ports={"6379": 6379},`,
			`    volumes={"./tmp/redis": "/data"},`,
			`    command=["redis-server", "--appendonly", "yes"],`,
			`)`,
			"",
			`prepared_api = container.create(`,
			`    name="payments-api",`,
			`    image="ghcr.io/babelsuite/payments-api:latest",`,
			`    after=["redis-cache"],`,
			`    env={"REDIS_ADDR": "redis-cache:6379"},`,
			`)`,
			"",
			`shared_proxy = container.get(name="otel-collector", after=["payments-api"])`,
			"",
			"# Container object methods for setup, debugging, and teardown.",
			`prepared_api.copy(src="./fixtures/app.yaml", dest="/app/config/app.yaml")`,
			`probe = cache.exec(command=["redis-cli", "ping"])`,
			`recent_logs = cache.logs(tail=20)`,
			`cache_ip = cache.ip()`,
			`cache_port = cache.port(6379)`,
			`details = cache.inspect()`,
			`prepared_api.start()`,
			`prepared_api.stop(timeout=5)`,
			`prepared_api.delete(force=True)`,
			"",
			"# Native BabelSuite mocks come from the suite's mock/ folder.",
			`orders_mock = mock.serve(`,
			`    name="orders-mock",`,
			`    source="./mock/orders",`,
			`    after=["payments-api"],`,
			`)`,
			"",
			"# Native mock object methods focus on BabelSuite behavior, state, and previewability.",
			`orders_ready = orders_mock.wait_ready()`,
			`orders_url = orders_mock.url()`,
			`orders_logs = orders_mock.logs(tail=20)`,
			`orders_mock.reset_state()`,
			`orders_preview = orders_mock.preview(operation="get-order")`,
			"",
			"# External mock daemons belong to the service module, not the native mock module.",
			`catalog_compat = service.prism(`,
			`    name="catalog-compat",`,
			`    spec_path="./compat/prism/openapi.yaml",`,
			`    port=4010,`,
			`    after=["orders-mock"],`,
			`)`,
			"",
			`legacy_compat = service.custom(`,
			`    name="legacy-compat",`,
			`    command=["node", "./scripts/custom_mock.js"],`,
			`    port=9090,`,
			`    after=["catalog-compat"],`,
			`)`,
			"",
			"# External service object methods cover daemon lifecycle and debug access.",
			`catalog_compat.wait_ready(endpoint="/health", timeout=10)`,
			`catalog_url = catalog_compat.url()`,
			`catalog_logs = catalog_compat.logs(tail=20)`,
			`catalog_compat.stop()`,
			`legacy_compat.kill()`,
			"",
			"# Scripts are short-lived synchronous tasks that must finish before the suite continues.",
			"# script.file(...) is the primary file-based task form; script.bash(...) stays as convenience sugar.",
			`bootstrap = script.file(`,
			`    name="bootstrap",`,
			`    file_path="./scripts/bootstrap.sh",`,
			`    interpreter="bash",`,
			`    env={"APP_ENV": "local"},`,
			`    after=["legacy-compat"],`,
			`)`,
			"",
			`migrate = script.sql_migrate(`,
			`    name="migrate-db",`,
			`    target="db",`,
			`    sql_dir="./sql",`,
			`    after=["bootstrap"],`,
			`)`,
			"",
			`seed = script.exec(`,
			`    name="seed-data",`,
			`    command=["make", "seed"],`,
			`    cwd="./scripts",`,
			`    env={"SEED_PROFILE": "local"},`,
			`    after=["migrate-db"],`,
			`)`,
			`seed.assert_success()`,
			`seed_exit_code = seed.exit_code`,
			`seed_stdout = seed.stdout`,
			`seed_stderr = seed.stderr`,
			"",
			"# Scenarios act as the attacker layer and compile the resulting test report.",
			`smoke = scenario.go(`,
			`    name="checkout-smoke",`,
			`    test_dir="./scenarios/go",`,
			`    objectives=["checkout", "payments"],`,
			`    tags=["smoke", "ci"],`,
			`    env={"BASE_URL": "http://payments-api:8080", "ORDERS_URL": orders_url},`,
			`    after=["payments-api", "orders-mock", "catalog-compat", "legacy-compat", "seed-data"],`,
			`)`,
			`smoke_passed = smoke.passed`,
			`smoke_exit_code = smoke.exit_code`,
			`smoke_duration_ms = smoke.duration_ms`,
			`smoke_logs = smoke.logs`,
			`smoke_summary = smoke.summary`,
			`smoke_failed = smoke.summary.failed`,
			`smoke_artifacts = smoke.artifacts_dir`,
		}
		return strings.Join(lines, "\n") + "\n"
	}

	loadSymbol := moduleLoadSymbol(module)
	name := moduleDirectoryName(module)

	lines := []string{
		fmt.Sprintf("load(%q, %q)", module.Title, loadSymbol),
		"",
		moduleInvocation(module, loadSymbol, name),
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderModuleExtraFiles(module catalog.Package) []RenderedFile {
	if moduleDirectoryName(module) != "runtime" {
		return nil
	}

	return []RenderedFile{
		{
			Path: "suite_example.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "container", "mock", "service", "script", "scenario")`,
				"",
				`redis = container.run(`,
				`    name="redis-cache",`,
				`    image="redis:alpine",`,
				`    ports={"6379": 6379},`,
				`)`,
				"",
				`api = container.run(`,
				`    name="payments-api",`,
				`    image="ghcr.io/acme/payments:latest",`,
				`    after=["redis-cache"],`,
				`    env={"REDIS_ADDR": "redis-cache:6379"},`,
				`)`,
				"",
				`orders_mock = mock.serve(`,
				`    name="orders-mock",`,
				`    source="./mock/orders",`,
				`    after=["payments-api"],`,
				`)`,
				"",
				`catalog_compat = service.prism(`,
				`    name="catalog-compat",`,
				`    spec_path="./compat/prism/openapi.yaml",`,
				`    port=4010,`,
				`    after=["orders-mock"],`,
				`)`,
				"",
				`seed = script.file(name="seed-data", file_path="./scripts/bootstrap.sh", interpreter="bash", after=["payments-api"])`,
				`migrate = script.sql_migrate(name="migrate-db", target="db", sql_dir="./sql", after=["seed-data"])`,
				`smoke = scenario.go(name="checkout-smoke", test_dir="./scenarios/go", objectives=["checkout"], tags=["smoke"], after=["migrate-db", "catalog-compat"])`,
			}, "\n"),
		},
		{
			Path: "scripts/container_lifecycle.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "container")`,
				"",
				`cache = container.get(name="redis-cache")`,
				`probe = cache.exec(command=["redis-cli", "ping"])`,
				`if probe.exit_code != 0:`,
				`    print(cache.logs(tail=100))`,
				"",
				`cache_ip = cache.ip()`,
				`cache_port = cache.port(6379)`,
				`details = cache.inspect()`,
				"",
				`api = container.create(`,
				`    name="payments-api",`,
				`    image="ghcr.io/acme/payments:latest",`,
				`    env={"REDIS_ADDR": "redis-cache:6379"},`,
				`)`,
				`api.copy(src="./fixtures/app.yaml", dest="/app/config/app.yaml")`,
				`api.start()`,
				`api.stop(timeout=5)`,
				`api.delete(force=True)`,
			}, "\n"),
		},
		{
			Path: "scripts/mock_lifecycle.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "mock")`,
				"",
				`orders = mock.serve(`,
				`    name="orders-mock",`,
				`    source="./mock/orders",`,
				`)`,
				"",
				`orders.wait_ready()`,
				`orders_url = orders.url()`,
				`orders_logs = orders.logs(tail=50)`,
				`orders.reset_state()`,
				`preview = orders.preview(operation="get-order")`,
			}, "\n"),
		},
		{
			Path: "scripts/service_lifecycle.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "service")`,
				"",
				`catalog = service.prism(`,
				`    name="catalog-compat",`,
				`    spec_path="./compat/prism/openapi.yaml",`,
				`    port=4010,`,
				`)`,
				"",
				`legacy = service.custom(`,
				`    name="legacy-compat",`,
				`    command=["node", "./scripts/custom_mock.js"],`,
				`    port=9090,`,
				`    after=["catalog-compat"],`,
				`)`,
				"",
				`catalog.wait_ready(endpoint="/health", timeout=10)`,
				`catalog_url = catalog.url()`,
				`catalog_logs = catalog.logs(tail=50)`,
				`catalog.stop()`,
				`legacy.kill()`,
			}, "\n"),
		},
		{
			Path: "scripts/script_results.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "script")`,
				"",
				`bootstrap = script.file(`,
				`    name="bootstrap",`,
				`    file_path="./scripts/bootstrap.sh",`,
				`    interpreter="bash",`,
				`    env={"APP_ENV": "local"},`,
				`)`,
				`bootstrap.assert_success()`,
				`bootstrap_exit = bootstrap.exit_code`,
				`bootstrap_stdout = bootstrap.stdout`,
				`bootstrap_stderr = bootstrap.stderr`,
				"",
				`quick_bootstrap = script.bash(name="quick-bootstrap", file_path="./scripts/bootstrap.sh", after=["bootstrap"])`,
				`quick_bootstrap.assert_success()`,
				"",
				`migrate = script.sql_migrate(`,
				`    name="migrate-db",`,
				`    target="db",`,
				`    sql_dir="./sql",`,
				`    after=["quick-bootstrap"],`,
				`)`,
				`migrate.assert_success()`,
				"",
				`seed = script.exec(`,
				`    name="seed-data",`,
				`    command=["make", "seed"],`,
				`    cwd="./scripts",`,
				`    env={"SEED_PROFILE": "local"},`,
				`    after=["migrate-db"],`,
				`)`,
				`seed.assert_success()`,
				`seed_stdout = seed.stdout`,
			}, "\n"),
		},
		{
			Path: "scenario_reports.star",
			Content: strings.Join([]string{
				`load("@babelsuite/runtime", "scenario")`,
				"",
				`go_suite = scenario.go(`,
				`    name="checkout-go",`,
				`    test_dir="./scenarios/go",`,
				`    objectives=["checkout", "payments"],`,
				`    tags=["smoke", "ci"],`,
				`    env={"BASE_URL": "http://payments-api:8080"},`,
				`)`,
				`go_passed = go_suite.passed`,
				`go_exit_code = go_suite.exit_code`,
				`go_duration_ms = go_suite.duration_ms`,
				`go_logs = go_suite.logs`,
				`go_summary = go_suite.summary`,
				`go_failed = go_suite.summary.failed`,
				`go_artifacts = go_suite.artifacts_dir`,
				"",
				`python_suite = scenario.python(`,
				`    name="checkout-python",`,
				`    test_dir="./scenarios/python",`,
				`    objectives=["checkout"],`,
				`    tags=["regression"],`,
				`    env={"BASE_URL": "http://payments-api:8080"},`,
				`    after=["checkout-go"],`,
				`)`,
				`python_passed = python_suite.passed`,
				`python_exit_code = python_suite.exit_code`,
				`python_duration_ms = python_suite.duration_ms`,
				`python_summary = python_suite.summary`,
				"",
				`http_suite = scenario.http(`,
				`    name="checkout-http",`,
				`    collection_path="./scenarios/http/checkout.hurl",`,
				`    objectives=["edge"],`,
				`    tags=["api"],`,
				`    env={"BASE_URL": "http://payments-api:8080"},`,
				`    after=["checkout-python"],`,
				`)`,
				`http_passed = http_suite.passed`,
				`http_exit_code = http_suite.exit_code`,
				`http_duration_ms = http_suite.duration_ms`,
				`http_logs = http_suite.logs`,
				`http_summary = http_suite.summary`,
			}, "\n"),
		},
		{
			Path: "scripts/custom_mock.js",
			Content: strings.Join([]string{
				`const http = require("http")`,
				``,
				`const port = process.env.PORT || 9090`,
				``,
				`const server = http.createServer((req, res) => {`,
				`  res.writeHead(200, { "Content-Type": "application/json" })`,
				`  res.end(JSON.stringify({ source: "custom-mock", path: req.url }))`,
				`})`,
				``,
				`server.listen(port, "127.0.0.1", () => {`,
				`  console.log("custom mock listening on " + port)`,
				`})`,
			}, "\n"),
		},
		{
			Path: "scripts/bootstrap.sh",
			Content: strings.Join([]string{
				`#!/usr/bin/env bash`,
				`set -euo pipefail`,
				`echo "bootstrapping local suite assets"`,
			}, "\n"),
		},
		{
			Path: "mock/orders/get-order.cue",
			Content: strings.Join([]string{
				`"$schema": "https://schemas.babelsuite.dev/mock-exchange-source-v1.json"`,
				`suite: "runtime-module-example"`,
				`artifact: "mock/orders/get-order.cue"`,
				`operationId: "get-order"`,
				`adapter: "rest"`,
				`dispatcher: "apisix"`,
				`examples: {`,
				`  "approved": {`,
				`    dispatch: [{from:"path", param:"id", value:"ord_123"}]`,
				`    responseSchema: {`,
				`      status: "200"`,
				`      mediaType: "application/json"`,
				`      body: {`,
				`        id: "ord_123"`,
				`        status: "approved"`,
				`        traceId: string @gen(kind="uuid")`,
				`      }`,
				`    }`,
				`  }`,
				`}`,
			}, "\n"),
		},
		{
			Path: "sql/001_init.sql",
			Content: strings.Join([]string{
				`create table if not exists orders (`,
				`  id text primary key,`,
				`  status text not null`,
				`);`,
			}, "\n"),
		},
		{
			Path: "compat/wiremock/mappings/get-order.json",
			Content: strings.Join([]string{
				`{`,
				`  "request": {`,
				`    "method": "GET",`,
				`    "urlPath": "/orders/ord_123"`,
				`  },`,
				`  "response": {`,
				`    "status": 200,`,
				`    "headers": {`,
				`      "Content-Type": "application/json"`,
				`    },`,
				`    "jsonBody": {`,
				`      "id": "ord_123",`,
				`      "status": "approved"`,
				`    }`,
				`  }`,
				`}`,
			}, "\n"),
		},
		{
			Path: "compat/prism/openapi.yaml",
			Content: strings.Join([]string{
				`openapi: 3.0.3`,
				`info:`,
				`  title: Catalog Mock`,
				`  version: 1.0.0`,
				`paths:`,
				`  /products:`,
				`    get:`,
				`      operationId: listProducts`,
				`      responses:`,
				`        "200":`,
				`          description: ok`,
				`          content:`,
				`            application/json:`,
				`              schema:`,
				`                type: array`,
				`                items:`,
				`                  type: object`,
				`                  properties:`,
				`                    sku:`,
				`                      type: string`,
				`                    name:`,
				`                      type: string`,
			}, "\n"),
		},
		{
			Path: "fixtures/app.yaml",
			Content: strings.Join([]string{
				"app:",
				"  name: payments-api",
				"  cacheUrl: redis://redis-cache:6379",
				"  logLevel: debug",
			}, "\n"),
		},
		{
			Path: "scenarios/go/checkout_test.go",
			Content: strings.Join([]string{
				`package checkout`,
				``,
				`import "testing"`,
				``,
				`func TestCheckoutSmoke(t *testing.T) {`,
				`	baseURL := "http://payments-api:8080"`,
				`	if baseURL == "" {`,
				`		t.Fatal("expected a base URL from the scenario environment")`,
				`	}`,
				`}`,
			}, "\n"),
		},
		{
			Path: "scenarios/python/test_checkout.py",
			Content: strings.Join([]string{
				`import os`,
				``,
				``,
				`def test_checkout_smoke():`,
				`    assert os.environ.get("BASE_URL")`,
			}, "\n"),
		},
		{
			Path: "scenarios/http/checkout.hurl",
			Content: strings.Join([]string{
				`GET {{BASE_URL}}/health`,
				`HTTP 200`,
			}, "\n"),
		},
	}
}

func moduleLoadSymbol(module catalog.Package) string {
	switch moduleDirectoryName(module) {
	case "runtime":
		return "container"
	case "postgres":
		return "pg"
	default:
		return moduleDirectoryName(module)
	}
}

func moduleInvocation(module catalog.Package, loadSymbol, name string) string {
	switch moduleDirectoryName(module) {
	case "postgres":
		return `db = pg(name="db")`
	case "kafka":
		return `broker = kafka(name="kafka")`
	default:
		return fmt.Sprintf(`%s_instance = %s(name=%q)`, strings.ReplaceAll(name, "-", "_"), loadSymbol, name)
	}
}

func moduleDirectoryName(module catalog.Package) string {
	repository := strings.TrimSpace(module.Repository)
	if repository == "" {
		return strings.TrimPrefix(strings.TrimSpace(module.Title), "@babelsuite/")
	}
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	return parts[len(parts)-1]
}

func ensureTrailingNewline(content string) string {
	if strings.HasSuffix(content, "\n") {
		return content
	}
	return content + "\n"
}

func joinPath(parts ...string) string {
	return filepath.ToSlash(filepath.Join(parts...))
}
