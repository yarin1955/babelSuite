---
title: Getting Started
---

# Getting Started

[Back to index](index.md)

## What You Run

BabelSuite has three primary executables:

| Binary | Path | Purpose |
|--------|------|---------|
| `babelsuite` server | `backend/cmd/server` | Control plane API (port 8090) |
| `babelsuite` agent | `backend/cmd/agent` | Remote worker (port 8091) |
| `babelctl` | `backend/cmd/ctl` | CLI client |

The frontend lives in `frontend/` and runs as a separate Vite dev server on port 5173.

## Prerequisites

- **Go** — backend binaries
- **Node.js** — frontend dev server
- **MongoDB** or **PostgreSQL** — primary datastore (MongoDB is the default)
- **Docker** — local container execution and environment inventory
- **Redis** *(optional)* — cache layer and faster runtime coordination

The checked-in `.env` defaults to MongoDB + Redis.

## Local Development

### 1. Configure the environment

The repo root includes a `.env` file. Common local settings:

```bash
PORT=8090
FRONTEND_URL=http://localhost:5173
VITE_API_URL=http://localhost:8090
DB_DRIVER=mongo
MONGO_URI=mongodb://localhost:27017
MONGO_DB=babelsuite
PLATFORM_SETTINGS_FILE=configuration.yaml
PROFILES_FILE=babelsuite-profiles.yaml
BABELSUITE_ENABLE_DEMO=false
```

See [Configuration](configuration.md) for the full variable reference.

### 2. Start the backend

From the `backend/` directory:

```bash
go run ./cmd/server
```

The server listens on `http://localhost:8090`.

### 3. Start the frontend

From the `frontend/` directory:

```bash
npm install
npm run dev
```

Vite serves the UI at `http://localhost:5173`.

### 4. Start a remote worker (optional)

If you want to test remote-agent execution:

```bash
go run ./cmd/agent
```

The worker process exposes its own health and execution API and registers with the control plane.

## Default Local Login

!!! note "Seeded admin account"
    The backend seeds an initial admin account on first startup. Use these credentials to sign in locally:

    - **Email:** `admin@babelsuite.test`
    - **Password:** `admin`

## First Things To Try

After signing in:

1. Open `/catalog` to browse registry-discovered packages.
2. Open `/suites` to inspect the runnable suite packages.
3. Open `/profiles` to view suite launch profiles.
4. Launch a suite from the home page or suites page.
5. Watch live events and logs on `/executions/:executionId`.
6. Open `/environments` to inspect active runtime resources.

## Example Suites

The repository includes workspace suites under `examples/oci-suites/`:

| Suite | Description |
|-------|-------------|
| `payment-suite` | Service + mock + test topology for a payment flow |
| `identity-broker` | Auth and identity service composition |
| `returns-control-plane` | Returns and refund workflow |
| `storefront-browser-lab` | Browser-based end-to-end setup |
| `soap-claims-hub` | SOAP/legacy service integration |
| `fleet-control-room` | Fleet management control plane |
| `composite-readiness` | Nested suite composition example |

## Optional: Local Registry Content

To work with catalog discovery and local OCI package flows, populate the local Zot registry:

```bash
# From backend/
go run ./cmd/seed-zot
```

!!! tip
    The `tools/` directory contains additional scripts for configuring and running a local Zot OCI registry. See [Examples](examples.md) for the full setup guide.
