---
title: Configuration
---

# Configuration

[Back to index](index.md)

## Main Configuration Sources

BabelSuite currently uses three main configuration layers:

- `.env` for process settings
- `configuration.yaml` for platform settings
- `babelsuite-profiles.yaml` for stored profile records managed through the API

## `.env`

The repo root `.env` covers:

### Application

- `PORT`
- `FRONTEND_URL`
- `VITE_API_URL`
- `JWT_SECRET`
- `ADMIN_EMAIL`
- `ADMIN_PASSWORD`

### Datastore

- `DB_DRIVER`
- `MONGO_URI`
- `MONGO_DB`
- `POSTGRES_DSN`

### Platform files

- `PLATFORM_SETTINGS_FILE`
- `PROFILES_FILE`
- `AGENT_RUNTIME_FILE`

### Cache

- `REDIS_ADDR`
- `REDIS_PASSWORD`
- `REDIS_DB`
- `REDIS_PREFIX`
- `CACHE_TTL_*`

### Demo mode

- `BABELSUITE_ENABLE_DEMO`

### Local auth and SSO

- `AUTH_PASSWORD_LOGIN_ENABLED`
- `AUTH_SIGNUP_ENABLED`
- `OIDC_ENABLED`
- `OIDC_PROVIDER_ID`
- `OIDC_PROVIDER_NAME`
- `OIDC_ISSUER_URL`
- `OIDC_CLIENT_ID`
- `OIDC_CLIENT_SECRET`
- `OIDC_REDIRECT_URL`
- `OIDC_FRONTEND_CALLBACK_URL`
- `OIDC_SCOPES`
- `OIDC_PKCE_ENABLED`
- `OIDC_EMAIL_CLAIM`
- `OIDC_NAME_CLAIM`
- `OIDC_GROUPS_CLAIM`
- `OIDC_ADMIN_GROUPS`
- `AUTH_STATE_SECRET`

### Telemetry

- `OTEL_EXPORTER_OTLP_ENDPOINT`
- `OTEL_SERVICE_NAME`
- `OTEL_EXPORTER_OTLP_INSECURE`
- `OTEL_EXPORTER_OTLP_HEADERS`
- `OTEL_RESOURCE_ATTRIBUTES`

## `configuration.yaml`

The platform settings file defines the control plane’s physical execution environment.

Current top-level sections:

- `mode`
- `description`
- `agents`
- `registries`
- `secrets`

### Agents

An agent entry defines one execution target, for example:

- local Docker
- remote worker pool
- Kubernetes runner

Important fields include:

- `agentId`
- `name`
- `type`
- `enabled`
- `default`
- `routingTags`
- `dockerSocket`
- `hostUrl`
- `kubeconfigPath`
- `targetNamespace`

### Registries

Registry entries drive the catalog.

Important fields include:

- `registryId`
- `name`
- `provider`
- `registryUrl`
- `repositoryScope`
- `syncStatus`

### Secrets

The secrets section defines the shared secrets provider and global overrides.

Important fields include:

- `provider`
- `vaultAddress`
- `vaultNamespace`
- `vaultRole`
- `awsRegion`
- `secretPrefix`
- `globalOverrides`

## Demo Mode vs Workspace Mode

If `BABELSUITE_ENABLE_DEMO=true`:

- demo files under `demo/` are used

If `BABELSUITE_ENABLE_DEMO=false`:

- suites come from `examples/oci-suites/`
- platform settings come from `configuration.yaml`
- the app behaves like a real workspace/control-plane setup

## Backend Selection

Execution backends come from platform settings. If no enabled platform backends are available, the execution service falls back to a local backend binding.

## Recommended Local Defaults

For a local workstation setup:

- MongoDB as the primary datastore
- Redis enabled if available
- one default local execution agent
- one local registry entry for package discovery
- local auth enabled
- OIDC off until an issuer is available
