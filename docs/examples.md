---
title: Examples
---

# Examples

[Back to index](index.md)

## Example Suites

The repository currently includes these workspace suites under `examples/oci-suites/`:

- `composite-readiness`
- `fleet-control-room`
- `identity-broker`
- `kafka-topic-lifecycle`
- `payment-suite`
- `returns-control-plane`
- `soap-claims-hub`
- `storefront-browser-lab`

These examples cover different kinds of suite composition:

- browser-driven flows
- async and event-driven topologies
- mock-heavy API suites
- nested suite composition
- policy and fixture-driven tests

## Example Modules

The example module folders under `examples/oci-modules/` are:

- `kafka`
- `postgres`

These are pure Starlark module examples built on top of the built-in runtime primitives.

## Syncing Example Content

The repository includes:

- `backend/cmd/sync-examples`

This command syncs generated example workspace content into the checked-in examples root.

Run it from `backend/` with:

```powershell
go run ./cmd/sync-examples
```

## Seeding A Local Registry

The repository also includes:

- `backend/cmd/seed-zot`

This command can publish seeded references into a local registry so the catalog has discoverable package content.

## Why The Examples Matter

The examples act as:

- runnable reference suites
- UI inspection data
- catalog enrichment sources
- authoring examples for new suites and modules
