---
title: Profiles
---

# Profiles

[Back to index](index.md)

## What Profiles Do

Profiles are the launch-time configuration layer for a suite. They allow the same suite topology to run differently across environments — local development, CI, staging, canary, or event-specific scenarios.

A profile can:

- set environment variables for the run
- select module hints and observability settings
- override individual service env vars
- reference secrets from an external provider
- inherit from another profile via `extendsId`

## Profile Sources

BabelSuite has two profile representations:

| Source | Location | Purpose |
|--------|----------|---------|
| **Workspace files** | `examples/oci-suites/<suite>/profiles/*.yaml` | Package-owned defaults; travel with the suite |
| **Managed records** | `babelsuite-profiles.yaml` / profiles API | Operator-managed overlays, secret bindings, UI-editable |

Workspace files drive the initial profile list for each suite. Managed records support creation, updates, deletion, and default selection through the UI and API.

## Workspace Profile Shape

Common checked-in profile shape:

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

The workspace loader uses `name`, `description`, `default`, `runtime.repository`, `runtime.profileFile`, and `modules` from this structure.

## Managed Profile Record Fields

Profiles stored via the API expose:

| Field | Description |
|-------|-------------|
| `id` | Record identifier |
| `name` | Display name |
| `fileName` | YAML filename (must end in `.yaml` or `.yml`) |
| `description` | Optional description |
| `scope` | Suite scope this profile belongs to |
| `yaml` | The profile YAML payload (validated on write) |
| `secretRefs` | List of secret references (`key`, `provider`, `ref`) |
| `default` | Whether this is the default profile for the suite |
| `extendsId` | ID of a parent profile to inherit from |
| `launchable` | Whether this profile appears in the launch UI |
| `updatedAt` | Last modified timestamp |

The `yaml` payload typically carries:

```yaml
env:
  LOG_LEVEL: debug
  TELEMETRY_PROFILE: verbose
services:
  api:
    env:
      API_MODE: strict
```

## Default Selection

At launch time:

1. The explicitly chosen profile wins.
2. Otherwise the profile marked `default: true` is used.
3. Otherwise the first available profile is used.

## Suite Dependency Profiles

Nested suite dependencies can declare their own runtime profile in `dependencies.yaml`:

```yaml
dependencies:
  auth-module:
    ref: localhost:5000/core-platform/identity-broker
    version: 1.2.0
    profile: canary.yaml
```

That selected profile travels with the imported suite's resolved topology and step specs. The resolver reads the child suite's `profiles/canary.yaml` and applies its `env:` and per-service `services.<step>.env:` overlays to imported nodes before execution.

## Managed Profile Endpoints

| Method | Endpoint | Action |
|--------|----------|--------|
| `GET` | `/api/v1/profiles/suites` | List suite profile summaries |
| `GET` | `/api/v1/profiles/suites/:suiteId` | Get profiles for one suite |
| `POST` | `/api/v1/profiles/suites/:suiteId` | Create a profile |
| `PUT` | `/api/v1/profiles/suites/:suiteId/:profileId` | Update a profile |
| `DELETE` | `/api/v1/profiles/suites/:suiteId/:profileId` | Delete a profile |
| `POST` | `/api/v1/profiles/suites/:suiteId/:profileId/default` | Mark as default |

See [API](api.md) for the full route reference.

## Related Pages

- [Profile Runtime Reference](profile-runtime.md) — workspace vs managed profiles in depth, runtime overlays, dependency profile flow
- [Suites](suites.md) — how profiles relate to suite packages and launch options
- [Dependency Manifests](dependencies.md) — how dependency `profile` and `inputs` travel with nested suites
