---
title: Dependency Manifests Reference
---

# Dependency Manifests Reference

[Back to index](index.md)

## Why Dependency Manifests Exist

Nested suite composition separates:

- orchestration in `suite.star`
- dependency resolution in `dependencies.yaml`
- exact resolved artifacts in `dependencies.lock.yaml`

This keeps the suite graph readable and keeps refs, versions, and digests in one place.

## `dependencies.yaml`

The manifest supports two shapes.

### Scalar Form

```yaml
dependencies:
  payments-module: localhost:5000/core-platform/payment-suite:workspace
```

### Object Form

```yaml
dependencies:
  payments-module:
    ref: localhost:5000/core-platform/payment-suite
    version: 1.2.0
    profile: local.yaml
    inputs:
      DATABASE_URL: postgres://postgres:postgres@global-db:5432/payments
      STRIPE_BASE_URL: http://stripe-mock:8080
```

## `dependencies.lock.yaml`

The lock file records the exact resolved artifact that should be used.

```yaml
locks:
  payments-module:
    version: 1.2.0
    resolved: localhost:5000/core-platform/payment-suite@sha256:1111111111111111111111111111111111111111111111111111111111111111
    digest: sha256:1111111111111111111111111111111111111111111111111111111111111111
    profile: local.yaml
    inputs:
      DATABASE_URL: postgres://postgres:postgres@global-db:5432/payments
```

The lock entry can also be a simple scalar:

```yaml
locks:
  payments-module: localhost:5000/core-platform/payment-suite@sha256:1111111111111111111111111111111111111111111111111111111111111111
```

## Resolution Rules

The resolver currently enforces these rules:

- every `suite.run(ref="alias")` alias must exist in `dependencies.yaml`
- `latest` is rejected
- a dependency must be pinned by version or digest
- a lock file can supply the exact resolved digest
- version mismatches between `ref:` and `version:` are rejected
- the referenced suite must exist in the current suite catalog

## Runtime Expansion

Imported suites are flattened into one final topology before execution.

Example:

```star
payments = suite.run(ref="payments-module", after=[global_db])
```

Imported nodes become namespaced:

- `payments/api`
- `payments/migrate`
- `payments/checkout-smoke`

That namespacing prevents collisions with parent-suite step names.

## Dependency Runtime Overlays

Dependency entries can carry two runtime overlays:

- `profile`
- `inputs`

Those values travel with the imported nodes and become part of their runtime metadata.

### Profile

The `profile` must exist in the imported suite's `profiles/` folder.

If present, it becomes the imported node's `runtimeProfile`.

### Inputs

`inputs` are merged into the imported node runtime environment.

For imported nodes, BabelSuite currently adds metadata variables such as:

- `BABELSUITE_DEPENDENCY_ALIAS`
- `BABELSUITE_DEPENDENCY_SUITE`
- `BABELSUITE_DEPENDENCY_REF`
- `BABELSUITE_PROFILE`
- `BABELSUITE_DEPENDENCY_RESOLVED`
- `BABELSUITE_DEPENDENCY_DIGEST`

If the imported node is a mock, it also gets:

- `x-suite-profile`

## Dependency Profile Runtime Files

When a dependency selects a profile, the resolver reads that imported suite profile file and can apply:

- global `env:`
- `services.<step>.env:`

to the imported suite's namespaced runtime nodes.

Example imported profile:

```yaml
name: Local
env:
  JWT_AUDIENCE: payments
services:
  api:
    env:
      API_MODE: strict
```

That runtime information is merged into the imported topology before execution.

## Recommended Practice

- Pin versions explicitly.
- Commit `dependencies.lock.yaml`.
- Avoid `latest`.
- Keep `suite.star` focused on aliases like `suite.run(ref="payments-module")`.
- Use dependencies for reusable subsystems, not tiny helper functions.
