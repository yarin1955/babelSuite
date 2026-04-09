---
title: BabelSuite
---

# BabelSuite

BabelSuite is an open-source control plane for running Starlark-defined suites made of services, tasks, tests, traffic phases, mocks, and nested suite composition.

Each suite describes a topology graph of background services, one-shot tasks, verification tests, traffic steps, and nested suites. BabelSuite resolves that graph, launches steps against a local, Kubernetes, or remote-agent backend, and streams live events and logs to the UI in real time.

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Suite** | The runnable package. Contains `suite.star`, profiles, metadata, contracts, mocks, services, tasks, tests, traffic plans, and resources. |
| **Profile** | Launch-time configuration overlay — selects env vars, modules, and observability settings for a run. |
| **Execution** | One run of a suite under a selected profile and backend. |
| **Backend** | Where steps run: `local`, `kubernetes`, or `remote-agent`. |
| **Catalog** | Registry-backed inventory of discoverable suite packages. |
| **Environment** | Live runtime inventory of containers, networks, and volumes from active runs. |
| **Agent** | Remote worker that registers with the control plane, claims work, streams logs, and completes jobs. |
| **Dependency Manifest** | Suite-level file (`dependencies.yaml`) that maps nested suite aliases to pinned refs, versions, digests, profiles, and inputs. |

## Documentation Map

### Start Here

- [Getting Started](getting-started.md) — prerequisites, local dev flow, first things to try

### System

- [Architecture](architecture.md) — system layers, control plane composition, data flows, storage model
- [Control Plane Reference](control-plane.md) — middleware, request IDs, tracing, audit, health internals
- [Configuration](configuration.md) — all `.env` variables, `configuration.yaml`, demo vs workspace mode
- [Platform Settings](platform.md) — agents, registries, and secrets model

### Suites and Authoring

- [Suites](suites.md) — suite structure, topology families, nested suites, dependency rules
- [Suite Authoring Reference](suite-authoring.md) — package layout, recognized folders, naming advice
- [Dependency Manifests](dependencies.md) — `dependencies.yaml` and `dependencies.lock.yaml` in depth
- [Runtime Library Reference](runtime-library.md) — built-in Starlark surface: `service`, `task`, `test`, `traffic`, and `suite`

### Profiles and Mocking

- [Profiles](profiles.md) — profile sources, shape, API records, default selection
- [Profile Runtime Reference](profile-runtime.md) — workspace vs managed profiles, runtime overlays, dependency profile flow
- [Mocking](mocking.md) — mock endpoints, operation metadata, fallback modes, stateful mocking
- [Mocking Reference](mocking-reference.md) — complete field reference for surfaces, operations, state, and exchanges

### Execution and Infrastructure

- [Modules](modules.md) — built-in runtime vs OCI example modules (Kafka, Postgres)
- [Execution](execution.md) — launch model, backends, step spec, live streams, remote agents
- [Agents](agents.md) — worker lifecycle, control plane endpoints, worker process endpoints, payloads
- [Environments](environments.md) — runtime inventory model, SSE updates, cleanup operations
- [Catalog](catalog.md) — OCI discovery, package fields, favorites

### Interfaces, Examples, and Operations

- [Authentication](auth.md) — local auth, OIDC flow, JWT session model
- [API](api.md) — full HTTP API route reference
- [CLI](cli.md) — `babelctl` commands and usage examples
- [Examples](examples.md) — example suite packages and local registry setup
- [Development](development.md) — local dev commands, test, sync, seed
- [Operations](operations.md) — health/readiness probes, telemetry, cache, datastores

## Product Surface

| Route | Purpose |
|-------|---------|
| `/` | Home dashboard — execution overview |
| `/catalog` | Registry-backed package discovery |
| `/suites` | Runnable suite explorer |
| `/profiles` | Suite profile management |
| `/executions/:executionId` | Live execution detail with event and log streams |
| `/environments` | Runtime inventory and cleanup |
| `/settings/*` | Platform configuration (admin only) |
| `/sign-in`, `/sign-up`, `/auth/callback` | Authentication |

## Repository Layout

```text
backend/           Go control plane, remote worker, CLI, and all internal services
frontend/          React application (TypeScript + Vite)
examples/
  oci-suites/      Runnable suite packages
  oci-modules/     Pure Starlark example modules (kafka, postgres)
proto/             API service definitions
demo/              Demo-mode data files
tools/             Local helper scripts and configuration
docs/              This documentation (MkDocs Material)
```

## Running The Docs Locally

```bash
pip install -r docs/requirements.txt
mkdocs serve
```

The local site is available at `http://127.0.0.1:8000/`.

## Publishing

The repository includes a documentation deployment workflow at `.github/workflows/docs.yml`.

To enable GitHub Pages:

1. Open repository **Settings → Pages**.
2. Set the source to the `gh-pages` branch, root folder.
3. Save.

Pushes to `main` that change `docs/**` or `mkdocs.yml` will then auto-deploy the site.
