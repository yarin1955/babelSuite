---
title: Profile Runtime Reference
---

# Profile Runtime Reference

[Back to index](index.md)

## Two Profile Layers

BabelSuite currently has two profile representations:

1. workspace profile files under `profiles/*.yaml`
2. managed profile records in `backend/suite-profiles.yaml` or the profile store API

Workspace files drive suite discovery and default launch choices. Managed profiles support UI/API management, inheritance, defaults, and secret references.

## Workspace Profile Shape

The common checked-in profile shape looks like this:

```yaml
name: Local Debug
description: Verbose logs, local secrets, and relaxed timeouts.
default: true
runtime:
  suite: payment-suite
  repository: localhost:5000/core-platform/payment-suite
  profileFile: local.yaml
modules:
  - postgres
  - kafka
observability:
  logs: structured
  traces: enabled
  metrics: enabled
```

The workspace loader currently uses these fields directly:

- `name`
- `description`
- `default`
- `runtime.repository`
- `runtime.profileFile`
- `modules`

## Managed Profile Record Shape

Managed profiles stored through the API expose fields such as:

- `id`
- `name`
- `fileName`
- `description`
- `scope`
- `yaml`
- `secretRefs`
- `default`
- `extendsId`
- `launchable`
- `updatedAt`

The managed YAML payload is validated as YAML and can carry fields like:

- `env`
- `services`

Example:

```yaml
env:
  LOG_LEVEL: debug
  TELEMETRY_PROFILE: verbose
services:
  uiPort: 13000
  dispatcherPort: 18081
```

## Defaults And Selection

At launch time:

- the explicitly chosen profile wins
- otherwise the default profile is used when one exists
- otherwise the first available profile is used

## Dependency Profile Runtime

The clearest runtime profile flow today is on imported nested suites.

When a dependency manifest selects a child profile:

```yaml
dependencies:
  auth-module:
    ref: localhost:5000/core-platform/identity-broker
    version: 1.2.0
    profile: local.yaml
```

the resolver:

- verifies that `local.yaml` exists in the child suite
- reads `profiles/local.yaml` from that suite
- applies child `env:` and `services.<step>.env:` to imported nodes
- sets the imported node `runtimeProfile`
- adds `x-suite-profile` for imported mock nodes

## Current Runtime Metadata Fields

Execution step payloads can carry:

- `profile`
- `runtimeProfile`
- `env`
- `headers`

This is especially important for nested suites, where the imported runtime overlay is resolved before the step reaches the selected backend.

## Validation Rules

Managed profile records enforce:

- `name` is required
- `fileName` is required
- `fileName` must end in `.yaml` or `.yml`
- `yaml` is required and must parse
- file names must be unique within a suite
- `extendsId` must point at an existing profile when set
- secret refs need `key`, `provider`, and `ref`

## Practical Guidance

- Use workspace profiles for package-owned defaults that should travel with the suite.
- Use managed profiles for operator-managed overlays and secret bindings.
- Use dependency `profile` when importing a suite that needs its own internal launch context.
- Keep profile file names stable, because they become runtime selectors and dependency references.
