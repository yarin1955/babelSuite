---
title: Operations
---

# Operations

[Back to index](index.md)

## Health Endpoints

The control plane exposes:

- `GET /healthz`
- `GET /readyz`
- `GET /readyz/{subsystem}`
- `GET /api/v1/system/healthz`
- `GET /api/v1/system/readyz`
- `GET /api/v1/system/readyz/{subsystem}`

### Liveness

`/healthz` is the simple process-level check.

### Readiness

`/readyz` returns a JSON report covering subsystem checks such as:

- database
- cache
- platform settings
- profiles
- telemetry
- agents
- launchable suites

Required subsystems can make readiness fail with `503 Service Unavailable`.

## Middleware And Request Discipline

The shared HTTP stack adds:

- CORS handling
- request IDs
- session context population
- tracing hooks
- HTTP metrics
- audit middleware

This keeps cross-cutting behavior consistent across the API surface.

## Telemetry

OpenTelemetry can be enabled with the OTLP environment variables in `.env`.

Common settings:

- `OTEL_EXPORTER_OTLP_ENDPOINT`
- `OTEL_SERVICE_NAME`
- `OTEL_EXPORTER_OTLP_INSECURE`
- `OTEL_EXPORTER_OTLP_HEADERS`
- `OTEL_RESOURCE_ATTRIBUTES`

If telemetry is not configured, the readiness report marks that subsystem as disabled instead of hard failing the server.

## Cache Layer

Redis is optional. When configured, it is used for:

- cached reads
- favorites and workspace acceleration
- execution runtime cache
- platform and profile cache

If Redis is missing, the control plane can still run, but the readiness report will reflect the cache state.

## Datastores

The primary datastore can be:

- MongoDB
- PostgreSQL

MongoDB is the default local path in the checked-in `.env`.

## Environment Inventory

The environments page is backed by a runtime inventory service that:

- polls Docker resources
- tracks orchestrator process liveness
- identifies zombie environments
- supports SSE updates
- allows per-environment or global cleanup

## Worker Health

The worker process exposes its own `GET /healthz` endpoint and a small runtime API for:

- info
- run
- cancel
- cleanup

That lets a control plane or external operator verify worker availability independently.
