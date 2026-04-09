---
title: Mocking Reference
---

# Mocking Reference

[Back to index](index.md)

## Mocking Model

BabelSuite's mock layer combines:

- API surface metadata
- operation metadata
- exchange examples
- fallback behavior
- optional in-memory state transitions

The suite hydration layer also normalizes mock metadata paths and runtime URLs so the UI and runtime can inspect them consistently.

## API Surface Fields

An API surface currently exposes fields such as:

- `id`
- `title`
- `protocol`
- `mockHost`
- `description`
- `operations`

## Operation Fields

Each operation can carry:

- `id`
- `method`
- `name`
- `summary`
- `contractPath`
- `mockPath`
- `mockUrl`
- `curlCommand`
- `dispatcher`
- `mockMetadata`
- `exchanges`

## Mock Metadata Fields

Per-operation mock metadata supports:

- `adapter`
- `dispatcher`
- `dispatcherRules`
- `delayMillis`
- `parameterConstraints`
- `fallback`
- `state`
- `metadataPath`
- `resolverUrl`
- `runtimeUrl`

## Parameter Constraints

Each parameter constraint can define:

- `name`
- `source`
- `required`
- `forward`
- `pattern`

## Fallback Modes

Fallback metadata supports:

- static inline fallback bodies
- named example fallbacks
- proxy fallback URLs

Fields include:

- `mode`
- `exampleName`
- `proxyUrl`
- `status`
- `mediaType`
- `body`
- `headers`

## Stateful Mocking

Mock state can define:

- `lookupKeyTemplate`
- `mutationKeyTemplate`
- `defaults`
- `transitions`

Each transition can define:

- `onExample`
- `mutationKeyTemplate`
- `set`
- `delete`
- `increment`

This supports behaviors like create-update-delete flows, sequence state, and request-driven mutations.

## Exchange Example Fields

Each exchange example can include:

- `name`
- `sourceArtifact`
- `when`
- `requestHeaders`
- `requestBody`
- `responseStatus`
- `responseMediaType`
- `responseHeaders`
- `responseBody`

Each `when` condition can define:

- `from`
- `param`
- `value`

## Protocol Endpoints

The backend currently exposes mock routes for:

- `GET /mocks/rest/...`
- `POST /mocks/grpc/{suiteId}/{surfaceId}/{operationId}`
- `POST /mocks/async/{suiteId}/{surfaceId}/{operationId}`

There is also an internal resolver path rooted under:

- `/internal/mock-data/`

## Defaulting During Hydration

During suite hydration:

- missing mock adapters are derived from protocol
- missing dispatchers are defaulted
- metadata paths are inferred from `mockPath`
- resolver URLs are generated
- runtime URLs are generated
- schema-backed mock references are normalized

## Authoring Guidance

- Keep contracts in `api/` and mock artifacts in `mock/`.
- Prefer named exchange examples over giant inline bodies in `suite.star`.
- Use fallback behavior intentionally so the mock surface degrades predictably.
- Use state transitions only when the suite really needs request-driven stateful behavior.
