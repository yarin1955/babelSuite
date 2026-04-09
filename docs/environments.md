---
title: Environments
---

# Environments

[Back to index](index.md)

## What The Environments Page Shows

The environments page is the runtime inventory and cleanup view for managed resources.

It shows:

- tracked environments
- containers
- networks
- volumes
- CPU and memory usage
- zombie detection
- cleanup actions

## Frontend Route And API

The frontend route is:

- `/environments`

The backing API currently still uses the `sandboxes` path:

- `GET /api/v1/sandboxes`
- `GET /api/v1/sandboxes/events`
- `POST /api/v1/sandboxes/reap-all`
- `POST /api/v1/sandboxes/{sandboxId}/reap`

## Inventory Model

An environment snapshot contains:

- `dockerAvailable`
- `updatedAt`
- `summary`
- `sandboxes`
- `warnings`

Each tracked environment can include:

- `sandboxId`
- `runId`
- `suite`
- `owner`
- `profile`
- `status`
- `summary`
- `startedAt`
- `lastHeartbeatAt`
- `orchestratorPid`
- `orchestratorState`
- `isZombie`
- `canReap`
- `resourceUsage`
- `containers`
- `networks`
- `volumes`
- `warnings`

## Live Updates

The page can subscribe to a server-sent event stream from:

- `/api/v1/sandboxes/events`

That stream replays the latest snapshot and then publishes changes over time.

## Cleanup

Cleanup supports:

- one environment at a time
- all tracked environments at once

The cleanup result reports how many containers, networks, and volumes were removed.
