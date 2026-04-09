---
title: API
---

# API

[Back to index](index.md)

## Route Groups

The control plane API is grouped by responsibility.

## System

- `GET /healthz`
- `GET /readyz`
- `GET /readyz/{subsystem}`
- `GET /api/v1/system/healthz`
- `GET /api/v1/system/readyz`
- `GET /api/v1/system/readyz/{subsystem}`

## Authentication

- `GET /api/v1/auth/config`
- `POST /api/v1/auth/sign-up`
- `POST /api/v1/auth/sign-in`
- `GET /api/v1/auth/me`
- `GET /api/v1/auth/sso/providers`
- `GET /api/v1/auth/oidc/login`
- `GET /api/v1/auth/oidc/callback`

## Catalog

- `GET /api/v1/catalog/packages`
- `GET /api/v1/catalog/packages/{packageId}`
- `GET /api/v1/catalog/favorites`
- `POST /api/v1/catalog/favorites/{packageId}`
- `DELETE /api/v1/catalog/favorites/{packageId}`

## Suites

- `GET /api/v1/suites`
- `GET /api/v1/suites/{suiteId}`

## Profiles

- `GET /api/v1/profiles/suites`
- `GET /api/v1/profiles/suites/{suiteId}`
- `POST /api/v1/profiles/suites/{suiteId}`
- `PUT /api/v1/profiles/suites/{suiteId}/{profileId}`
- `DELETE /api/v1/profiles/suites/{suiteId}/{profileId}`
- `POST /api/v1/profiles/suites/{suiteId}/{profileId}/default`

## Executions

- `GET /api/v1/executions/launch-suites`
- `GET /api/v1/executions/overview`
- `GET /api/v1/executions`
- `POST /api/v1/executions`
- `GET /api/v1/executions/{executionId}`
- `GET /api/v1/executions/{executionId}/events`
- `GET /api/v1/executions/{executionId}/logs`

## Engine

- `GET /api/v1/engine/overview`
- `GET /api/v1/engine/overview/stream`

## Platform Settings

- `GET /api/v1/platform-settings`
- `PUT /api/v1/platform-settings`
- `POST /api/v1/platform-settings/registries/{registryId}/sync`

## Environments

The frontend route is `/environments`, but the backing API currently uses `/api/v1/sandboxes`:

- `GET /api/v1/sandboxes`
- `GET /api/v1/sandboxes/events`
- `POST /api/v1/sandboxes/reap-all`
- `POST /api/v1/sandboxes/{sandboxId}/reap`

## Agent Registry And Control Plane

- `GET /api/v1/agents`
- `POST /api/v1/agents/register`
- `POST /api/v1/agents/{agentId}/heartbeat`
- `DELETE /api/v1/agents/{agentId}`
- `POST /api/v1/agent-control/claims/next`
- `POST /api/v1/agent-control/jobs/{jobId}/lease`
- `POST /api/v1/agent-control/jobs/{jobId}/state`
- `POST /api/v1/agent-control/jobs/{jobId}/logs`
- `POST /api/v1/agent-control/jobs/{jobId}/complete`

## Worker Runtime

The standalone worker process exposes:

- `GET /healthz`
- `GET /api/v1/agent/info`
- `POST /api/v1/agent/run`
- `POST /api/v1/agent/jobs/{jobId}/cancel`
- `POST /api/v1/agent/jobs/{jobId}/cleanup`

## Mocks

- `/internal/mock-data/`
- `/mocks/rest/`
- `POST /mocks/grpc/{suiteId}/{surfaceId}/{operationId}`
- `POST /mocks/async/{suiteId}/{surfaceId}/{operationId}`

## Authentication Requirement

Most application routes are protected. Streaming routes allow session lookup from the query token so the browser can connect to SSE endpoints.
