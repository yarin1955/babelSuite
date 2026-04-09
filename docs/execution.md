---
title: Execution
---

# Execution

[Back to index](index.md)

## Launch Model

An execution is one run of a suite under:

- one selected profile
- one selected (or auto-detected) backend
- one resolved flat topology

```
POST /api/v1/executions
  { suiteId, profile, backend? }
       │
       ▼
 resolve topology
       │
       ▼
 select backend
       │
       ▼
 convert nodes → step specs
       │
       ▼
 dispatch to backend ──► stream events + logs over SSE
```

The execution service keeps:

- execution summaries (for the overview list)
- detailed execution records (per-run metadata)
- step snapshots (per-step status and progress)
- events (timestamped status transitions)
- log lines (per-step output)

## Launchable Suites

The launch API exposes suites as `LaunchSuite` objects, each with:

- suite identity (id, title, repository)
- profile options available for that suite
- backend options from platform settings

This ensures the UI shows only suites that can actually be launched.

## Backends

The execution service resolves the backend from platform settings:

| Kind | Description |
|------|-------------|
| `local` | Runs steps via the local Docker daemon |
| `kubernetes` | Dispatches steps to a configured Kubernetes cluster |
| `remote-agent` | Routes work to a registered remote worker process |

If platform settings are missing or empty, the service falls back to a local binding automatically.

## Step Spec

Each topology node becomes a step spec that carries:

- execution and suite metadata
- selected profile and runtime profile
- env vars and request headers
- backend identity
- source suite identity (for imported nested-suite nodes)
- dependency alias, resolved ref, and digest
- step index and total step count
- full node definition

## Live Streams

Executions expose two SSE endpoints:

| Endpoint | Content |
|----------|---------|
| `GET /api/v1/executions/:id/events` | Status change events per step |
| `GET /api/v1/executions/:id/logs` | Log line stream per step |

The live execution page subscribes to both streams so the UI updates without polling.

!!! note
    SSE connections pass the session token as a query parameter (`?token=...`) because the browser `EventSource` API does not support custom request headers.

## Execution Overview

The engine maintains a rolling in-memory overview of recent and active executions:

- `GET /api/v1/engine/overview` — current snapshot
- `GET /api/v1/engine/overview/stream` — SSE stream of overview updates

The home dashboard uses the SSE stream to display live execution status.

## Remote Agents

When the selected backend is `remote-agent`, the coordinator assigns work to registered workers:

1. Control plane queues the step as a pending assignment
2. Worker polls `POST /agent-control/claims/next` to claim work
3. Worker extends leases via `POST /agent-control/jobs/:id/lease`
4. Worker streams state and logs back to the control plane
5. Worker marks the job complete via `POST /agent-control/jobs/:id/complete`

See [Agents](agents.md) for the full worker lifecycle and payload reference.

## Environment Inventory

While executions run, Docker resources (containers, networks, volumes) are tracked on the environments page at `/environments`. The backend API path for this is `/api/v1/sandboxes`.

See [Environments](environments.md) for the inventory model and cleanup operations.

## Health And Readiness

The control plane exposes readiness checks around execution dependencies:

- `datastore` — primary store availability
- `cache` — Redis availability (optional)
- `platform-settings` — at least one backend configured
- `profiles` — profile store available
- `launchable-suites` — at least one suite is launchable

See [Operations](operations.md) for the full health endpoint reference.

## Related Pages

- [Agents](agents.md) — remote worker lifecycle and coordination API
- [Environments](environments.md) — runtime resource inventory and cleanup
- [Operations](operations.md) — health/readiness endpoints and telemetry
- [Suites](suites.md) — suite topology and how it maps to step specs
