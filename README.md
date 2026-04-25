<p align="center">
  <img src="docs/assets/logo.png" alt="BabelSuite" width="220" />
</p>

<h3 align="center">Container-native orchestrator for complex test suites and multi-language simulators</h3>

<p align="center">
  <a href="https://github.com/babelsuite/babelsuite/actions/workflows/docs.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/babelsuite/babelsuite/docs.yml?branch=main&label=docs&logo=github&style=flat-square" alt="Docs" />
  </a>
  <a href="https://github.com/babelsuite/babelsuite/releases/latest">
    <img src="https://img.shields.io/github/v/release/babelsuite/babelsuite?sort=semver&style=flat-square&logo=github" alt="Release" />
  </a>
  <a href="https://github.com/babelsuite/babelsuite/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/babelsuite/babelsuite?style=flat-square" alt="License" />
  </a>
  <a href="https://goreportcard.com/report/github.com/babelsuite/babelsuite">
    <img src="https://goreportcard.com/badge/github.com/babelsuite/babelsuite?style=flat-square" alt="Go Report Card" />
  </a>
  <a href="https://github.com/babelsuite/babelsuite/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22">
    <img src="https://img.shields.io/github/issues-search/babelsuite/babelsuite?query=is%3Aopen+label%3A%22good+first+issue%22&label=good+first+issues&style=flat-square&color=7057ff" alt="Good First Issues" />
  </a>
</p>

<p align="center">
  <a href="https://babelsuite.github.io/babelsuite"><strong>Documentation</strong></a> ·
  <a href="https://babelsuite.github.io/babelsuite/getting-started"><strong>Getting Started</strong></a> ·
  <a href="https://babelsuite.github.io/babelsuite/examples"><strong>Examples</strong></a> ·
  <a href="https://github.com/babelsuite/babelsuite/discussions"><strong>Discussions</strong></a>
</p>

---

BabelSuite is an open-source control plane for running **Starlark-defined suites** made of services, tasks, tests, traffic phases, mocks, and nested suite composition. It resolves dependency graphs, launches steps against a local Docker, Kubernetes, or remote-agent backend, and streams live events and logs to the UI in real time.

## Why BabelSuite?

Modern integration testing requires spinning up multiple services, applying mock contracts, seeding databases, running traffic phases, and tearing everything down cleanly — in the right order, with full observability. BabelSuite encodes that entire lifecycle in a single `suite.star` file and executes it end-to-end.

## Features

- **Starlark suite definitions** — expressive, Python-like topology scripts with built-in `service`, `task`, `test`, `traffic`, and `suite` primitives
- **DAG-based execution** — dependency graph resolved via topological sort; steps launch in correct order and in parallel where safe
- **Multi-backend** — run steps against local Docker, a Kubernetes cluster, or a pool of remote agents
- **Live event streaming** — real-time SSE updates of step status, logs, and execution transitions
- **Stateful service mocking** — mock endpoints with per-operation metadata, state machines, fallback modes, and example exchanges
- **Nested suite composition** — import other suites as pinned OCI dependencies with version, digest, and profile resolution
- **OCI registry integration** — browse and pull suite packages from any OCI-compliant registry (Zot, GHCR, ECR, etc.)
- **Profile system** — launch-time configuration overlays: select env vars, modules, and observability settings per run
- **Remote agent workers** — distributed execution with heartbeat coordination and log streaming
- **OpenTelemetry built-in** — traces and metrics exported via OTLP for backend and frontend
- **Multi-database** — MongoDB or PostgreSQL primary store; Redis optional cache layer
- **OIDC + local auth** — SSO via any OIDC provider, or plain email/password for local development

## Quick Start

**Prerequisites:** Go, Node.js, MongoDB (or PostgreSQL), Docker

```bash
# Clone the repository
git clone https://github.com/babelsuite/babelsuite.git
cd babelsuite

# Start the control plane (port 8090)
cd backend && go run ./cmd/server

# In another terminal, start the frontend (port 5173)
cd frontend && npm install && npm run dev
```

Open `http://localhost:5173` and sign in with `admin@babelsuite.test` / `admin`.

Seed the local OCI registry to get catalog packages:

```bash
cd backend && go run ./cmd/seed-zot
```

See the [Getting Started guide](https://babelsuite.github.io/babelsuite/getting-started) for the full walkthrough.

## How It Works

A suite is a `suite.star` file that declares a topology graph:

```python
def main(ctx):
    db = service(
        name="postgres",
        image="babelsuite/postgres:latest",
    )

    api = service(
        name="payment-api",
        image="acme/payment-api:latest",
        depends_on=[db],
    )

    mock = service(
        name="fraud-mock",
        variant="service.mock",
        depends_on=[api],
    )

    test(
        name="payment-flow",
        image="acme/payment-tests:latest",
        depends_on=[api, mock],
    )
```

BabelSuite resolves the dependency graph, starts each step in order, streams logs to the UI, and marks the execution healthy or failed based on the outcome.

## Architecture

```
Browser (React UI)
     │  REST + SSE
     ▼
Control Plane :8090
  ├── Auth / Session
  ├── Suite Service        ← loads and resolves suite.star topology
  ├── Execution Service    ← launches steps, streams events
  ├── Catalog Service      ← OCI registry discovery
  ├── Profile Service      ← launch-time config overlays
  ├── Platform Settings    ← agents, registries, secrets
  └── Environment Inventory← live container/network tracking
     │
     ├── Local Docker backend
     ├── Kubernetes backend
     └── Remote Agent :8091

Storage: MongoDB / PostgreSQL  +  Redis (optional)
```

## Example Suites

The repository includes seven runnable example suites under `examples/oci-suites/`:

| Suite | Description |
|-------|-------------|
| `payment-suite` | Service + mock + test topology for a payment flow |
| `identity-broker` | Auth and identity service composition |
| `returns-control-plane` | Returns and refund workflow with state transitions |
| `storefront-browser-lab` | Browser-based end-to-end setup with Playwright |
| `soap-claims-hub` | SOAP/legacy service integration |
| `fleet-control-room` | Fleet management control plane with traffic phases |
| `composite-readiness` | Nested suite composition across multiple registries |

## Documentation

Full documentation: **[babelsuite.github.io/babelsuite](https://babelsuite.github.io/babelsuite)**

| Section | Description |
|---------|-------------|
| [Getting Started](https://babelsuite.github.io/babelsuite/getting-started) | Prerequisites, local dev flow, first things to try |
| [Architecture](https://babelsuite.github.io/babelsuite/architecture) | System layers, control plane, data flows |
| [Suite Authoring](https://babelsuite.github.io/babelsuite/suite-authoring) | Package layout, topology primitives, naming |
| [Runtime Library](https://babelsuite.github.io/babelsuite/runtime-library) | `service`, `task`, `test`, `traffic`, `suite` reference |
| [Mocking](https://babelsuite.github.io/babelsuite/mocking) | Mock endpoints, state machines, fallback modes |
| [Execution](https://babelsuite.github.io/babelsuite/execution) | Launch model, backends, step spec, live streams |
| [Agents](https://babelsuite.github.io/babelsuite/agents) | Remote worker lifecycle and coordination |
| [API](https://babelsuite.github.io/babelsuite/api) | Full HTTP API route reference |
| [CLI](https://babelsuite.github.io/babelsuite/cli) | `babelctl` commands and usage examples |
| [Operations](https://babelsuite.github.io/babelsuite/operations) | Health probes, telemetry, cache, datastores |

Run the docs locally:

```bash
pip install -r docs/requirements.txt
mkdocs serve
# → http://127.0.0.1:8000/
```

## Repository Layout

```
backend/           Go control plane, remote worker, CLI, and internal services
  cmd/
    server/        Control plane API (port 8090)
    agent/         Remote worker process (port 8091)
    ctl/           babelctl CLI
  internal/        All internal packages
frontend/          React application (TypeScript + Vite, port 5173)
examples/
  oci-suites/      Runnable example suite packages
  oci-modules/     Pure Starlark modules (kafka, postgres)
proto/             Protobuf API definitions
docs/              MkDocs Material documentation site
tools/             Local helper scripts (Zot registry, seeding, etc.)
```

## Contributing

We welcome contributions of all kinds. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

- **Bug reports** — [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml)
- **Feature requests** — [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml)
- **Questions** — [GitHub Discussions](https://github.com/babelsuite/babelsuite/discussions)
- **Security issues** — see [SECURITY.md](SECURITY.md) for responsible disclosure

All participants are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

BabelSuite is licensed under the [Apache License 2.0](LICENSE).
