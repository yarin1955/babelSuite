---
title: Suite Authoring Reference
---

# Suite Authoring Reference

[Back to index](index.md)

## Authoring Model

A suite is a filesystem package rooted at one folder under `examples/oci-suites/<suite-id>/`.

At startup, BabelSuite reads:

- `suite.star` for the topology graph
- `README.md` for title and description
- `metadata.yaml` for optional suite labels and tags
- `profiles/*.yaml` for launchable profile options
- `dependencies.yaml` and `dependencies.lock.yaml` for nested suite manifests
- recognized content folders such as `api/`, `mock/`, `services/`, `tasks/`, `tests/`, `traffic/`, and `resources/`

## Recommended Package Layout

```text
my-suite/
  README.md
  metadata.yaml
  suite.star
  dependencies.yaml
  dependencies.lock.yaml
  profiles/
    local.yaml
    staging.yaml
  api/
  mock/
  services/
  tasks/
  tests/
  traffic/
  resources/
    certs/
    data/
```

## Recognized Root Files

| File | Required | Purpose |
|------|----------|---------|
| `suite.star` | Yes | Topology entrypoint |
| `README.md` | No | Title and description |
| `metadata.yaml` | No | Suite labels, tags, and metadata |
| `dependencies.yaml` | No | Nested suite dependency manifest |
| `dependencies.lock.yaml` | No | Resolved dependency lock file |

## Recognized Folders

| Folder | Role | Purpose |
|--------|------|---------|
| `profiles/` | Configuration | Launch profiles and runtime metadata |
| `api/` | Contracts | Schemas and compatibility surface definitions |
| `mock/` | Mocking | Mock metadata, examples, and schema-backed artifacts |
| `services/` | Infrastructure | Background service assets and compatibility configs |
| `tasks/` | Operations | One-shot jobs, migrations, and seed scripts |
| `tests/` | Verification | Smoke, regression, browser, and protocol assertions |
| `traffic/` | Performance | Traffic plans and workload definitions |
| `resources/` | Assets | Passive data, certificates, and large static blobs |

Compatibility folders still load for older suites:

- `scripts/`
- `scenarios/`
- `fixtures/`
- `certs/`
- `policies/`

## How `suite.star` Is Parsed

The topology parser is an assignment-based topology scanner, not a full Starlark interpreter. It scans each logical statement for this shape:

```text
<variable> = <family>(<arguments>)
```

Statements that do not match this shape, including `load()` statements, comments, and blank lines, are ignored by the resolver.

Backslash continuations are supported for chained `.export(...)` calls.

### `load()` statements

`load()` lines are still expected at the top of each suite:

```python
load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")
load("@babelsuite/kafka", "kafka")
```

The topology parser ignores them. The workspace loader reads them separately to populate the suite's contracts/module references.

## Canonical Example

```python
load("@babelsuite/postgres", "pg")
load("@babelsuite/runtime", "service", "task", "test", "traffic")

db       = service.run()
stripe   = service.mock(after=[db])
migrate  = task.run(file="migrate.py", image="python:3.12", after=[db])
api      = service.run(after=[db, stripe, migrate])
baseline = traffic.baseline(plan="baseline.star", target="http://api:8080", after=[api])
smoke    = test.run(file="go/smoke_test.go", image="golang:1.24", after=[baseline])
```

## Recognized Topology Families

| Family | Preferred calls |
|--------|-----------------|
| `service` | `service.run`, `service.mock`, `service.wiremock`, `service.prism`, `service.custom` |
| `task` | `task.run` |
| `test` | `test.run` |
| `traffic` | `traffic.smoke`, `traffic.baseline`, `traffic.stress`, `traffic.spike`, `traffic.soak`, `traffic.scalability`, `traffic.step`, `traffic.wave`, `traffic.staged`, `traffic.constant_throughput`, `traffic.constant_pacing`, `traffic.open_model` |
| `suite` | `suite.run` |

The only retained legacy bridge is `mock.serve`, which still maps to `service.mock` while older suites are being migrated.

For the full runtime surface and per-family argument reference, see [Runtime Library Reference](runtime-library.md).

## Dependency Ordering

Use `after=[db, api]` to declare which nodes a given node depends on. The resolver builds a DAG from those edges and produces a topologically sorted execution order.

```python
db    = service.run()
seed  = task.run(file="seed.sh", image="bash:5.2", after=[db])
api   = service.run(after=[db, seed])
tests = test.run(file="smoke.py", image="python:3.12", after=[api])
```

You can also define explicit success/failure rules on runnable nodes:

```python
seed = task.run(
    file="seed.sh",
    image="bash:5.2",
    expect_exit=0,
    expect_logs="Task completed successfully",
    fail_on_logs="FATAL ERROR",
)
```

Supported controls:

- `expect_exit=<int>`
- `expect_logs="..."`
- `expect_logs=["...", "..."]`
- `fail_on_logs="..."`
- `fail_on_logs=["...", "..."]`
- `continue_on_failure=true`
- `reset_mocks=[billing_mock]`

For rollback or contingency branches, use `on_failure=[node]`:

```python
primary = task.run(file="deploy.sh", image="bash:5.2")
rollback = task.run(file="rollback.sh", image="bash:5.2", on_failure=[primary])
```

To clear stateful mocks before a verification step, use `reset_mocks=[...]` on the test node:

```python
billing_mock = service.mock()
clean_test = test.run(file="verify_billing.py", image="python:3.12", reset_mocks=[billing_mock], after=[billing_mock])
```

You can also attach artifact export rules to a node:

```python
smoke = test.run(file="go/smoke_test.go", image="golang:1.24", after=[api]) \
  .export("coverage/*.xml", name="go-coverage", on="always", format="cobertura") \
  .export("reports/junit.xml", name="go-tests", format="junit") \
  .export("logs/crash.dump", name="crash-debug", on="failure")
```

Use `format="junit"` when you want the execution view to show pass/fail counts instead of only a raw export label. Use `format="cobertura"` for coverage reports.

Resolver rules:

- duplicate `after` entries are deduplicated automatically
- every dependency target must exist in the graph
- dependency cycles are rejected
- final order is topologically sorted; nodes at the same level run in source order
- a hard failure stops unrelated branches, but any activated `on_failure=[...]` branch can still run

## File Resolution Rules

- `task.run(file="seed.sh")` resolves to `tasks/seed.sh`
- `task.run(file="db/migrate.py")` resolves to `tasks/db/migrate.py`
- `test.run(file="go/smoke_test.go")` resolves to `tests/go/smoke_test.go`
- `traffic.baseline(plan="checkout.star")` resolves to `traffic/checkout.star`

## Naming Advice

Assignment names become execution event sources, live log sources, and dependency targets in `after=[...]` arrays. For nested suites they also become namespace prefixes such as `payments-module/checkout-smoke`.

Good names:

- `db`
- `payment_gateway`
- `seed_routes`
- `checkout_smoke`

Avoid generic names such as `step1`, `thing`, `main`, or `node`.

## Workspace Metadata Extraction

| Source | Extracted value |
|--------|-----------------|
| `README.md` first non-empty line | Suite title |
| `README.md` second non-empty line | Suite description |
| `profiles/*.yaml` | Launch profile list |
| `load("...")` in `suite.star` | Contracts / module references |

## Authoring Tips

- keep `suite.star` focused on orchestration instead of large inline data blobs
- put contract files under `api/` and mock definitions under `mock/`
- keep bootstrap logic under `tasks/` instead of baking it into service start commands
- keep verification logic under `tests/`
- keep large static assets under `resources/`
- use profiles for launch-time differences instead of duplicating whole suites
- use `suite.run(ref="...")` for reusing entire subsystems; use modules for smaller building blocks
