---
title: Agents
---

# Agents

[Back to index](index.md)

## What Agents Are

Agents are remote workers that can execute suite steps outside the main control-plane process.

They are useful for:

- isolated workloads
- heavier resource requirements
- remote execution pools
- host separation from the control plane

## Worker Lifecycle

The current remote worker model includes:

1. registration
2. heartbeat
3. claim next job
4. extend lease
5. report state
6. report log lines
7. complete the job

## Control Plane Endpoints

Registration and runtime coordination endpoints include:

- `GET /api/v1/agents`
- `POST /api/v1/agents/register`
- `POST /api/v1/agents/{agentId}/heartbeat`
- `DELETE /api/v1/agents/{agentId}`
- `POST /api/v1/agent-control/claims/next`
- `POST /api/v1/agent-control/jobs/{jobId}/lease`
- `POST /api/v1/agent-control/jobs/{jobId}/state`
- `POST /api/v1/agent-control/jobs/{jobId}/logs`
- `POST /api/v1/agent-control/jobs/{jobId}/complete`

## Worker Process Endpoints

The worker process itself exposes:

- `GET /healthz`
- `GET /api/v1/agent/info`
- `POST /api/v1/agent/run`
- `POST /api/v1/agent/jobs/{jobId}/cancel`
- `POST /api/v1/agent/jobs/{jobId}/cleanup`

## Payloads

When the control plane assigns work, the step request includes:

- execution metadata
- suite metadata
- profile and runtime profile
- env and headers
- backend identity
- dependency alias
- resolved ref and digest
- step order and node details

## Backend Integration

Execution backends can route work to:

- local execution
- Kubernetes execution
- remote workers

Remote agents are surfaced in platform settings as `remote-agent` style entries and are also tracked at runtime through the agent registry.
