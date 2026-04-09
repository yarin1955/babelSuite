---
title: Modules
---

# Modules

[Back to index](index.md)

## What Modules Mean In This Repository

There are two different ideas that come up around modules:

1. **Built-in runtime helpers** — the core node families (`service`, `task`, `test`, `traffic`, `suite`) built into BabelSuite's topology parser
2. **Example OCI modules** — reusable pure Starlark packages checked into `examples/oci-modules/`

For the built-in topology families, see [Runtime Library Reference](runtime-library.md).

## Module References In `suite.star`

Every example suite declares its module dependencies with `load()` statements:

```python
load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")
load("@babelsuite/kafka", "kafka")
load("@babelsuite/postgres", "pg")
```

These path strings are extracted by the workspace loader and stored as the suite's `Contracts` field. The topology parser itself ignores `load()` lines — see [Runtime Library Reference](runtime-library.md) for details.

## Module Status

| Module path | OCI package | Status |
|-------------|-------------|--------|
| `@babelsuite/runtime` | built-in | Core topology families — always available |
| `@babelsuite/kafka` | `examples/oci-modules/kafka` | Available — documented below |
| `@babelsuite/postgres` | `examples/oci-modules/postgres` | Available — documented below |
| `@babelsuite/redis` | *(none)* | Referenced in examples, no package yet |
| `@babelsuite/playwright` | *(none)* | Referenced in examples, no package yet |

!!! note
    `@babelsuite/redis` is referenced in `fleet-control-room/suite.star` and `@babelsuite/playwright` in `storefront-browser-lab/suite.star`. Neither has a corresponding OCI module package in `examples/oci-modules/` yet. They appear in `Contracts` for those suites but no module helpers are currently provided.

## Kafka Module

**Path:** `@babelsuite/kafka` — `examples/oci-modules/kafka`

Files:

| File | Purpose |
|------|---------|
| `module.star` | Public entrypoint |
| `cluster.star` | Cluster lifecycle helpers |
| `admin.star` | Topic and group admin helpers |
| `_shared.star` | Internal shared utilities |
| `usage.star` | Usage examples |
| `module.yaml` | Module metadata |
| `README.md` | Documentation |

Exported helpers:

| Helper | Description |
|--------|-------------|
| `kafka` | Cluster node helper |
| `create_topic` | Create a Kafka topic |
| `delete_topic` | Delete a Kafka topic |
| `set_group_offset` | Set a consumer group offset |
| `disconnect` | Close the connection |

## Postgres Module

**Path:** `@babelsuite/postgres` — `examples/oci-modules/postgres`

Files:

| File | Purpose |
|------|---------|
| `module.star` | Public entrypoint |
| `cluster.star` | Cluster lifecycle helpers |
| `query.star` | Query execution helpers |
| `_shared.star` | Internal shared utilities |
| `usage.star` | Usage examples |
| `module.yaml` | Module metadata |
| `README.md` | Documentation |

Exported helpers:

| Helper | Description |
|--------|-------------|
| `pg` | Cluster node helper |
| `connect` | Open a connection |
| `query` | Execute a raw query |
| `insert` | Insert rows |
| `select` | Select rows |
| `delete` | Delete rows |
| `upsert` | Insert or update rows |

## Module Metadata

Each OCI module carries a `module.yaml` metadata file with:

- module ID
- title
- repository
- provider
- version
- entrypoint
- description
- pull command
- fork command

## Why Example Modules Matter

These packages show the intended module layering:

- built on top of core runtime primitives
- reusable across suites
- small enough to stay understandable
- split into focused Starlark files instead of one large entrypoint

## Recommended Layering

| Layer | Use for |
|-------|---------|
| Runtime library | Built-in topology node primitives |
| OCI module | Reusable Starlark building blocks (kafka, postgres, ...) |
| Suite | Runnable topologies |
| Suite dependency | Larger package composition via `suite.run(ref="...")` |
