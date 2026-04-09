---
title: Platform Settings
---

# Platform Settings

[Back to index](index.md)

## What The Platform Settings Own

Platform settings describe the physical execution environment for the control plane.

They currently cover:

- execution agents
- OCI registries
- secrets provider configuration
- global secret and environment overrides

The backend stores and serves this data through:

- `GET /api/v1/platform-settings`
- `PUT /api/v1/platform-settings`
- `POST /api/v1/platform-settings/registries/{registryId}/sync`

## File Source

The initial source is the YAML file referenced by:

- `PLATFORM_SETTINGS_FILE`

In the checked-in local setup, that points to:

- `configuration.yaml`

## Agents

Each agent record can represent:

- a local host backend
- a remote worker pool
- a Kubernetes-backed runner

Common fields:

- `agentId`
- `name`
- `type`
- `enabled`
- `default`
- `description`
- `routingTags`
- `dockerSocket`
- `hostUrl`
- `kubeconfigPath`
- `targetNamespace`
- `serviceAccountToken`

Runtime-oriented fields can also appear:

- `registeredAt`
- `lastHeartbeatAt`
- `runtimeCapabilities`

## Registries

Registry entries feed the catalog discovery layer.

Common fields:

- `registryId`
- `name`
- `provider`
- `registryUrl`
- `username`
- `secret`
- `repositoryScope`
- `region`
- `syncStatus`
- `lastSyncedAt`

## Secrets

The secrets section defines the control plane’s shared secret provider and global overrides.

Fields include:

- `provider`
- `vaultAddress`
- `vaultNamespace`
- `vaultRole`
- `awsRegion`
- `secretPrefix`
- `globalOverrides`

Each global override contains:

- `key`
- `value`
- `description`
- `sensitive`

## Frontend Pages

The settings UI is split into:

- `/settings`
- `/settings/general`
- `/settings/agents`
- `/settings/registries`
- `/settings/secrets`

These pages all read from the same platform settings API surface.

## Registry Sync

When the user triggers a registry sync, the backend updates the registry status through the platform store and then catalog discovery can reflect the reachable repositories and tags.
