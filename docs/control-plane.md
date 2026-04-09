---
title: Control Plane Reference
---

# Control Plane Reference

[Back to index](index.md)

## Bootstrap Shape

The control plane server is responsible for wiring:

- HTTP routing
- auth/session services
- datastore
- cache
- platform settings
- profile store
- suite source
- catalog discovery
- execution service
- environment inventory
- agent coordination
- telemetry and readiness reporting

## Shared HTTP Middleware

The HTTP stack applies a shared cross-cutting layer for:

- route pattern tracking
- request IDs
- session context
- tracing attributes
- HTTP metrics
- audit logging

This keeps request behavior consistent across product areas instead of each handler rolling its own setup.

## Request IDs

Each request can receive a request ID in server context. That ID is then reused by:

- trace attributes
- audit records
- handler-level error tracking

## Tracing

The tracing middleware enriches spans with attributes such as:

- `http.request_id`
- `http.route`
- `enduser.id`
- `enduser.workspace_id`
- `enduser.admin`

## HTTP Metrics

The shared HTTP metrics layer records:

- request counts
- active requests
- request duration in milliseconds

with route-aware attributes.

## Audit Events

The audit middleware emits structured audit records for API and auth traffic, including:

- request ID
- method
- route
- path
- status
- duration
- remote address
- user ID
- workspace ID

Health endpoints are intentionally excluded from audit noise.

## Health Endpoints

The control plane exposes:

- `GET /healthz`
- `GET /readyz`
- `GET /readyz/{subsystem}`
- `GET /api/v1/system/healthz`
- `GET /api/v1/system/readyz`
- `GET /api/v1/system/readyz/{subsystem}`

### Liveness

`/healthz` is the simple process-level heartbeat.

### Readiness

`/readyz` returns a JSON report with:

- overall status
- check time
- per-subsystem results

Each subsystem result includes:

- `name`
- `status`
- `ready`
- `required`
- `detail`
- `checkedAt`
- `durationMs`

Required subsystem failures return `503 Service Unavailable`.

## Common Readiness Domains

The control plane wiring currently reports readiness around areas such as:

- datastore
- cache
- platform settings
- profiles
- telemetry
- agents
- launchable suites

Exact enabled probes can vary with configuration.

## Cache Layer

Redis is optional. When configured, it supports:

- cached reads
- favorites and workspace acceleration
- execution runtime cache
- platform/profile cache support

If cache is disabled or unavailable, the control plane still runs, but readiness can report that state separately.

## Worker Coordination

The control plane also owns the worker coordination API for:

- registration
- heartbeat
- claim next job
- lease extension
- state reports
- log reports
- completion

For payload details, see [Agents](agents.md).

## Environment Inventory

The environments page is also part of the control-plane surface. It tracks:

- managed containers
- networks
- volumes
- orchestrator process state
- zombie environments
- SSE inventory updates
- cleanup actions

For the runtime inventory model, see [Environments](environments.md).
