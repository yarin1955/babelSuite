---
title: Suites
---

# Suites

[Back to index](index.md)

## What A Suite Is

A suite is the core runnable package in BabelSuite.

Each suite combines:

- a topology graph defined in `suite.star`
- launch profiles
- mocks, contracts, and API surface definitions
- background services
- one-shot tasks
- verification tests
- traffic plans
- passive resources

The suite service loads suites from:

- demo files under `demo/` when demo mode is enabled
- workspace folders under `examples/oci-suites/` when demo mode is disabled

## Standard Layout

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

The workspace loader assigns first-class meaning to `profiles/`, `api/`, `mock/`, `services/`, `tasks/`, `tests/`, `traffic/`, and `resources/`.

## `suite.star`

`suite.star` is authored as a Starlark-like file. BabelSuite recognizes these public runtime families:

| Family | Variants |
|--------|---------|
| `service` | `service.run`, `service.mock`, `service.wiremock`, `service.prism`, `service.custom` |
| `task` | `task.run` |
| `test` | `test.run` |
| `traffic` | `traffic.smoke`, `traffic.baseline`, `traffic.stress`, `traffic.spike`, `traffic.soak`, `traffic.scalability`, `traffic.step`, `traffic.wave`, `traffic.staged`, `traffic.constant_throughput`, `traffic.constant_pacing`, `traffic.open_model` |
| `suite` | `suite.run` |

Each node declares ordering with `after=[db, api]`.

Bare `service(...)`, `task(...)`, `test(...)`, `traffic(...)`, and `suite(...)` are not accepted. Use the explicit family variants.

The only retained legacy bridge is `mock.serve`, which still maps to `service.mock` while older suites are being migrated.

## Example

```python
load("@babelsuite/runtime", "service", "task", "test", "traffic")

db          = service.run()
stripe_mock = service.mock(after=[db])
migrate     = task.run(file="migrate.py", image="python:3.12", after=[db])
api         = service.run(after=[db, stripe_mock, migrate])
smoke       = test.run(file="go/smoke_test.go", image="golang:1.24", after=[api])
```

For the full runtime surface, see [Runtime Library Reference](runtime-library.md).

## Folder-to-API Parity

The public authoring model is intentionally aligned with the suite folder structure:

- `service.run(...)` owns background infrastructure
- `service.mock(...)` is backed by `api/` and `mock/`
- `task.run(file="...")` reads from `tasks/`
- `test.run(file="...")` reads from `tests/`
- `traffic.*(plan="...")` reads from `traffic/`
- `resources/` holds passive assets such as certificates and static datasets

That keeps the suite package unambiguous: authors do not need to guess where a file belongs.

## Nested Suites

BabelSuite supports suite composition through `suite.run(ref="...")`.

The dependency definition lives in `dependencies.yaml`:

```yaml
dependencies:
  payments-module:
    ref: localhost:5000/core-platform/payment-suite
    version: 1.2.0
    profile: local.yaml
    inputs:
      STRIPE_BASE_URL: http://stripe-mock:8080
```

The resolved artifact lives in `dependencies.lock.yaml`:

```yaml
locks:
  payments-module:
    version: 1.2.0
    resolved: localhost:5000/core-platform/payment-suite@sha256:1111...
    digest: sha256:1111...
    profile: local.yaml
```

The suite graph imports that alias:

```python
payments = suite.run(ref="payments-module", after=[db])
```

For the full dependency manifest format and resolution rules, see [Dependency Manifests](dependencies.md).

## Dependency Rules

The topology resolver enforces:

- every `suite.run(ref="alias")` alias must exist in `dependencies.yaml`
- `latest` is rejected
- dependencies must be pinned by version or digest
- lock files provide the exact resolved digest
- cycles are rejected
- duplicate `after` entries are normalized away

## Resolved Topology

When a nested suite is expanded, imported nodes are namespaced under their alias. For example, importing `payments-module` produces:

- `payments-module/db`
- `payments-module/migrate`
- `payments-module/api`
- `payments-module/checkout-smoke`

This prevents step name collisions while keeping source provenance visible.

## Workspace Metadata

The suite loader derives package metadata from:

- `README.md` first non-empty line -> title
- `README.md` second non-empty line -> description
- `metadata.yaml` -> optional tags and labels
- `profiles/*.yaml` -> launch profile list
- `load(...)` statements in `suite.star` -> contract and module references

## Example Suites In This Repository

- `payment-suite`
- `identity-broker`
- `returns-control-plane`
- `storefront-browser-lab`
- `soap-claims-hub`
- `fleet-control-room`
- `composite-readiness`

See [Examples](examples.md) for descriptions of each suite and instructions for seeding the local registry.

## Related Pages

- [Suite Authoring Reference](suite-authoring.md) -> package layout, naming advice, authoring patterns
- [Runtime Library Reference](runtime-library.md) -> complete Starlark runtime surface
- [Dependency Manifests](dependencies.md) -> full dependency manifest format and resolution rules
- [Profiles](profiles.md) -> launch-time configuration for suites
