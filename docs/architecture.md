---
title: Architecture
---

# Architecture

[Back to index](index.md)

## High-Level Shape

BabelSuite is split into four main layers:

1. **Frontend** — React UI for catalog, suites, profiles, executions, environments, and settings
2. **Control plane** — HTTP API server: auth, platform settings, profiles, catalog, suite resolution, execution orchestration
3. **Execution backends** — local Docker, Kubernetes, or remote agent
4. **Runtime infrastructure** — primary datastore, optional cache, telemetry pipeline, Docker

```mermaid
graph TB
    Browser["Browser\n(React UI)"]

    subgraph ControlPlane["Control Plane (port 8090)"]
        API["HTTP API"]
        Auth["Auth / Session"]
        Suites["Suite Service"]
        Exec["Execution Service"]
        Catalog["Catalog Service"]
        Profiles["Profile Service"]
        Platform["Platform Settings"]
        Sandbox["Environment Inventory"]
        AgentCoord["Agent Coordinator"]
    end

    subgraph Backends["Execution Backends"]
        Local["Local (Docker)"]
        K8s["Kubernetes"]
        RemoteAgent["Remote Agent\n(port 8091)"]
    end

    subgraph Storage["Storage"]
        DB[("MongoDB / PostgreSQL")]
        Redis[("Redis (optional)")]
        YAML["YAML files\n(platform, profiles)"]
    end

    OCI["OCI Registry\n(Zot / GHCR / etc.)"]

    Browser -->|"REST + SSE"| API
    API --> Auth
    API --> Suites
    API --> Exec
    API --> Catalog
    API --> Profiles
    API --> Platform
    API --> Sandbox
    API --> AgentCoord

    Exec --> Local
    Exec --> K8s
    Exec --> RemoteAgent

    Suites --> DB
    Auth --> DB
    Exec --> DB
    Exec --> Redis
    Platform --> YAML
    Platform --> Redis
    Profiles --> YAML
    Profiles --> Redis

    Catalog --> OCI
```

## Control Plane Composition

The control plane is assembled in:

- `backend/cmd/server/main.go`
- `backend/cmd/server/app.go`
- `backend/cmd/server/health.go`

The server wires together:

- authentication
- suite loading
- profile management
- catalog discovery
- execution orchestration
- environment inventory
- platform settings
- agent registry and assignment coordination
- telemetry, request middleware, and readiness probes

## Data Flows

### Suite resolution flow

```mermaid
sequenceDiagram
    participant UI
    participant SuiteService
    participant Workspace
    participant DepResolver

    UI->>SuiteService: GET /api/v1/suites/:id
    SuiteService->>Workspace: read suite.star, README.md, profiles/
    Workspace-->>SuiteService: raw source files
    SuiteService->>DepResolver: expand dependencies.yaml + .lock.yaml
    DepResolver-->>SuiteService: flattened + namespaced topology
    SuiteService-->>UI: hydrated Definition (topology, surfaces, profiles)
```

### Execution launch flow

```mermaid
sequenceDiagram
    participant UI
    participant ExecService
    participant SuiteService
    participant Backend

    UI->>ExecService: POST /api/v1/executions {suiteId, profile}
    ExecService->>SuiteService: resolve topology
    SuiteService-->>ExecService: flat step list
    ExecService->>Backend: dispatch steps (local / k8s / remote-agent)
    Backend-->>ExecService: step events + log lines
    ExecService-->>UI: SSE stream (events + logs)
```

### Remote worker flow

```mermaid
sequenceDiagram
    participant Agent
    participant ControlPlane

    Agent->>ControlPlane: POST /agents/register
    loop heartbeat
        Agent->>ControlPlane: POST /agents/:id/heartbeat
    end
    Agent->>ControlPlane: POST /agent-control/claims/next
    ControlPlane-->>Agent: StepRequest
    loop during execution
        Agent->>ControlPlane: POST /agent-control/jobs/:id/lease
        Agent->>ControlPlane: POST /agent-control/jobs/:id/logs
        Agent->>ControlPlane: POST /agent-control/jobs/:id/state
    end
    Agent->>ControlPlane: POST /agent-control/jobs/:id/complete
```

## Main Backend Domains

| Package | Responsibility |
|---------|---------------|
| `internal/auth` | Local auth, session handling, OIDC login, JWT issuance |
| `internal/platform` | Agents, registries, secrets, platform settings |
| `internal/catalog` | Registry discovery and catalog package views |
| `internal/suites` | Suite loading, topology resolution, nested suite expansion |
| `internal/profiles` | Suite profile CRUD and validation |
| `internal/execution` | Launch, orchestration, runtime state, SSE streams |
| `internal/agent` | Worker registry, coordinator, worker control APIs |
| `internal/sandbox` | Environment inventory and cleanup APIs |
| `internal/httpserver` | Middleware: request IDs, audit hooks, tracing context |
| `internal/store` | Datastore and cache abstractions (MongoDB, PostgreSQL, Redis) |

## Frontend Surface

The React app currently exposes:

- home dashboard
- catalog browser
- suites explorer
- profiles manager
- live execution page
- environments page
- settings pages for general, agents, registries, and secrets
- local auth and SSO callback pages

## Storage Model

### Primary store

The control plane uses either:

- MongoDB (default)
- PostgreSQL

Configured via `DB_DRIVER` environment variable.

### Cache layer

Redis is optional and accelerates:

- cached reads for platform settings, profiles, and catalog
- execution runtime state
- coordination fast paths

The primary store remains the authority for persisted application state.

### File-backed state

Platform settings and managed profiles are stored in YAML files on disk:

- `configuration.yaml` — agents, registries, secrets
- `babelsuite-profiles.yaml` — managed profile records

Both are optionally cached in Redis.

## Middleware Stack

The server wraps all API traffic with shared middleware in this order:

```
CORS → Request IDs → Auth Session → OTel Trace → HTTP Metrics → Audit
```

This keeps system-wide concerns centralized rather than re-implemented in each handler.

For full middleware details, see [Control Plane Reference](control-plane.md).
