---
title: Runtime Library Reference
---

# Runtime Library Reference

[Back to index](index.md)

## What The Runtime Library Is

The runtime library is BabelSuite's built-in topology surface. It defines the node families that `suite.star` can assemble into a topology graph.

This is separate from the checked-in example OCI modules such as `kafka` or `postgres`.

## How The Parser Works

`suite.star` is processed by an assignment-based topology scanner, not a full Starlark interpreter. The parser scans each logical statement for this shape:

```text
<variable> = <family>(<arguments>)
```

Only statements that match this assignment shape are recognized as topology nodes. `load()` statements, comments, and blank lines are ignored by the topology resolver.

Backslash continuations are supported for chained `.export(...)` calls, but the parser still follows the same assignment-driven model.

## `load()` Statements

`load()` statements appear at the top of every example `suite.star`:

```python
load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")
```

The topology parser ignores these lines. Their job is different:

- the workspace loader reads `load("...")` lines to extract module path strings such as `@babelsuite/runtime` and `@babelsuite/kafka`
- those paths are stored as the suite's contracts/module references
- the imported names are a documentation and authoring convention, not parse-time bindings

In short: write `load()` lines to declare intent and populate the contracts surface. The topology resolver does not need them to recognize node calls.

## Core Families

### `service`

Long-lived background infrastructure in the suite graph.

| Call | Kind |
|------|------|
| `service.run` | `service` |
| `service.wiremock` | `service` |
| `service.prism` | `service` |
| `service.custom` | `service` |

```python
db  = service.run()
api = service.run(after=[db])
```

Use `service.run` for first-party background dependencies such as databases, APIs, caches, and workers.

`service(...)` is intentionally rejected. Use one of the explicit `service.*` forms.

### `service.mock`

Native suite-defined mock surfaces.

| Call | Kind |
|------|------|
| `service.mock` | `mock` |

```python
orders = service.mock(after=[api])
```

`service.mock` is the runtime entrypoint for mocks backed by the suite's `api/` and `mock/` folders.

Compatibility aliases still parse:

- `mock.serve`

### `task`

Short-lived jobs: setup, seeding, migrations, and one-off operations.

| Call | Kind |
|------|------|
| `task.run` | `task` |

```python
migrate = task.run(file="migrate.py", image="python:3.12", after=[db])
seed    = task.run(file="seed.sh", image="bash:5.2", after=[migrate])
```

`task(...)` is intentionally rejected. Use `task.run(...)`.

`task.run` resolves `file=` relative to `tasks/`, so `file="seed.sh"` means `./tasks/seed.sh`.

### `test`

Verification, smoke checks, browser assertions, and pass/fail validation.

| Call | Kind |
|------|------|
| `test.run` | `test` |

```python
smoke = test.run(file="go/smoke_test.go", image="golang:1.24", after=[api])
```

`test(...)` is intentionally rejected. Use `test.run(...)`.

`test.run` resolves `file=` relative to `tests/`, so `file="go/smoke_test.go"` means `./tests/go/smoke_test.go`.

### `traffic`

Throughput, concurrency, and latency testing.

| Call | Kind |
|------|------|
| `traffic.smoke` | `traffic` |
| `traffic.baseline` | `traffic` |
| `traffic.stress` | `traffic` |
| `traffic.spike` | `traffic` |
| `traffic.soak` | `traffic` |
| `traffic.scalability` | `traffic` |
| `traffic.step` | `traffic` |
| `traffic.wave` | `traffic` |
| `traffic.staged` | `traffic` |
| `traffic.constant_throughput` | `traffic` |
| `traffic.constant_pacing` | `traffic` |
| `traffic.open_model` | `traffic` |

```python
perf = traffic.smoke(
    plan="smoke.star",
    target="http://api:8080",
    after=[api],
)
```

`traffic(...)` is intentionally rejected. Use one of the explicit `traffic.*` forms.

Each `traffic.*` node must declare:

- `plan="..."` pointing at a file under `traffic/`
- `target="..."` as an absolute base URL for the native HTTP executor

Recommended meanings:

- `traffic.smoke`: a tiny preflight run before heavier profiles
- `traffic.baseline`: normal expected concurrent user load
- `traffic.stress`: push beyond normal operating range
- `traffic.spike`: sudden step up in user count
- `traffic.soak`: long-running endurance pressure
- `traffic.scalability`: multi-agent or multi-backend expansion checks
- `traffic.step`: increase traffic in discrete blocks
- `traffic.wave`: oscillating high/low cycles
- `traffic.staged`: several named phases with different targets
- `traffic.constant_throughput`: cap requests per second independently of user count
- `traffic.constant_pacing`: fixed interval between iterations per user
- `traffic.open_model`: fixed arrival-rate style workload independent of response time

These suite-facing entrypoints work with the native traffic plan builders inside `traffic/*.star`, including:

- `traffic.plan`
- `traffic.user`
- `traffic.task`
- `traffic.get`
- `traffic.post`
- `traffic.stage`
- `traffic.stages`
- `traffic.constant`
- `traffic.between`
- `traffic.pacing`
- `traffic.threshold`

Current behavior:

- the selected traffic profile is preserved in topology metadata and execution step payloads
- `plan=` and `target=` are parsed into a structured traffic spec on the topology node
- the native plan builders define concrete users, tasks, waits, stages, and thresholds
- the native runner executes real guarded HTTP traffic for supported profiles instead of only emitting synthetic traffic logs
- the native runner now reports richer latency and throughput summaries, including `min`, `avg`, `max`, `p50`, `p90`, `p95`, `p99`, per-stage summaries, compact throughput timelines, and latency histograms

Native threshold metrics currently supported:

- `http.error_rate`
- `http.min_ms`
- `http.avg_ms`
- `http.max_ms`
- `http.p50_ms`
- `http.p90_ms`
- `http.p95_ms`
- `http.p99_ms`
- `latency.min_ms`
- `latency.avg_ms`
- `latency.max_ms`
- `latency.p50_ms`
- `latency.p90_ms`
- `latency.p95_ms`
- `latency.p99_ms`
- `throughput.avg_rps`
- `throughput.peak_rps`

Current limit:

- the native executor currently supports HTTP `traffic.get(...)` and `traffic.post(...)` tasks only
- `traffic.scalability` still requires a distributed executor and is rejected by the native runner
- strict safety caps keep the control plane from self-harming with oversized traffic plans

### `suite`

Imports another suite via `dependencies.yaml`.

| Call | Kind |
|------|------|
| `suite.run` | `suite` |

```python
payments = suite.run(ref="payments-module", after=[db])
```

`suite(...)` is intentionally rejected. Use `suite.run(...)`.

For nested suite manifests, see [Dependency Manifests](dependencies.md).

## Resolver Argument Extraction

The parser extracts these topology fields from recognized statements:

| Argument | Used for |
|----------|---------|
| left-hand assignment (`db = ...`) | default node identity |
| `name="..."` or `id="..."` or `name_or_id="..."` | optional identity override |
| `after=[db, api]` | dependency edges |
| `on_failure=[smoke]` | failure-trigger ordering edge |
| `file="..."` | task/test asset path |
| `plan="..."` | traffic plan file |
| `target="..."` | native traffic target |
| `ref="..."` | nested suite alias for `suite.run` |
| `expect_exit=0` | expected process exit code |
| `expect_logs="..."` or `expect_logs=["...", "..."]` | required log/output matches |
| `fail_on_logs="..."` or `fail_on_logs=["...", "..."]` | forbidden log/output matches |
| `continue_on_failure=true` | mark the node failed but let normal downstream nodes continue |

The variable name is the default node ID. Use `name=` or `id=` only when you need an explicit override.

## Artifact Exports

Nodes can attach artifact export rules with chained `.export(...)` calls:

```python
smoke = test.run(file="go/smoke_test.go", image="golang:1.24") \
  .export("coverage/*.xml", name="go-coverage", on="always", format="cobertura") \
  .export("reports/junit.xml", name="go-tests", format="junit") \
  .export("logs/crash.dump", name="crash-debug", on="failure")
```

Supported export arguments:

- first positional string or `path="..."` for the artifact path or glob
- `name="..."` for the exported artifact label
- `on="success" | "failure" | "always"` to control when the export should run
- `format="junit" | "cobertura"` for structured test and coverage summaries in the execution UI

Current behavior:

- export rules are parsed into topology metadata
- export rules flow into execution step payloads
- the local runner registers and reports the rules in step logs
- `format="junit"` exports are summarized into pass/fail counts in the live execution view
- `format="cobertura"` exports are recognized now, and coverage summaries appear once report content is collected

Current limit:

- this is still not a full artifact collector yet, so raw files from real workloads are not harvested today
- JUnit summaries are synthesized from the step result until a real collector is attached

## Evaluation Controls

`task.run(...)`, `test.run(...)`, and other runnable nodes can declare explicit success/failure expectations:

```python
seed = task.run(
    file="seed.sh",
    image="bash:5.2",
    expect_exit=0,
    expect_logs="Task completed successfully",
    fail_on_logs=["FATAL ERROR", "panic:"],
)
```

Supported controls:

- `expect_exit=<int>`
- `expect_logs="..."`
- `expect_logs=["...", "..."]`
- `fail_on_logs="..."`
- `fail_on_logs=["...", "..."]`
- `continue_on_failure=true`
- `on_failure=[primary]`
- `reset_mocks=[billing_mock]`

Current behavior:

- exit-code expectations are checked after the runner finishes the step
- log assertions are matched against the emitted step log stream
- `traffic.*` steps still use threshold metrics for latency and error budgets
- `continue_on_failure=true` keeps the suite running even when the node itself finishes as `failed`
- `on_failure=[primary]` activates rollback or contingency nodes only when one of the referenced nodes fails
- `reset_mocks=[billing_mock]` clears persisted mock state before a `test.run(...)` step starts
- once a hard failure happens, unrelated branches are skipped while activated failure-path branches continue

Current limit:

- the local and orchestrated runners still use BabelSuite's current simulated task/test execution path, so log assertions match the step log stream rather than a full streamed process stdout/stderr capture

## Authoring Rules

- one node assignment per logical statement
- backslash continuations are supported for chained `.export(...)` calls
- the left-hand assignment becomes the default node ID
- use `after=[db, api]` to declare ordering; omitting it means the node has no dependencies
- use `on_failure=[primary]` for rollback or contingency branches
- quoted `after=["db"]` still parses, but identifier references are the preferred style
- `task.run(file="...")` resolves from `tasks/`
- `test.run(file="...")` resolves from `tests/`
- `traffic.*(plan="...")` resolves from `traffic/`
- `ref=` is required for `suite.run`; the parser errors if it is missing
- duplicate `after` entries are deduplicated automatically
- dependency targets that do not exist in the graph produce a resolver error
- cycles are rejected

## Example Modules vs Runtime

The runtime library is compiled into BabelSuite. The checked-in example modules under `examples/oci-modules/` are separate pure Starlark packages built on top of this surface:

- `examples/oci-modules/kafka` -> `@babelsuite/kafka`
- `examples/oci-modules/postgres` -> `@babelsuite/postgres`

See [Modules](modules.md) for those package details.
