---
title: Mocking
---

# Mocking

[Back to index](index.md)

## What The Mocking Layer Does

BabelSuite's mocking service resolves suite-defined API surfaces and returns mock responses for REST, gRPC, and async operations.

For each incoming mock request, the service locates:

1. the target suite and API surface
2. the matching operation
3. the best-matching example or schema-backed example

## Mock Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /mocks/rest/...` | REST mock handler |
| `POST /mocks/grpc/{suiteId}/{surfaceId}/{operationId}` | gRPC mock handler |
| `POST /mocks/async/{suiteId}/{surfaceId}/{operationId}` | Async mock handler |
| `/internal/mock-data/` | Internal metadata resolver |

## Operation Metadata

Each mock operation can carry:

- **adapter** — protocol adapter (`rest`, `grpc`, `async`)
- **dispatcher** — dispatch strategy (`example`, `script`, `sequence`, `random`)
- **dispatcherRules** — rules used by the dispatcher
- **delayMillis** — simulated latency
- **parameterConstraints** — named constraints for routing by request attributes
- **fallback** — what to return when no example matches
- **state** — stateful behavior rules (lookup/mutation keys, transitions)

For the complete field reference, see [Mocking Reference](mocking-reference.md).

## Exchange Examples

Operations include named exchange examples with:

- `requestHeaders` and `requestBody`
- `responseStatus`, `responseMediaType`, `responseHeaders`, and `responseBody`
- optional `when` conditions — match by state, parameter value, or header

## Fallback Modes

When no example matches the incoming request, the fallback controls the response:

| Mode | Behavior |
|------|---------|
| `static` | Return an inline body defined in the operation metadata |
| `example` | Return a named example from the operation's exchange list |
| `proxy` | Forward the request to an upstream URL |

This keeps a suite useful even when the exact request scenario isn't covered by an example.

## Stateful Mocking

Operations can declare state rules to enable request-driven in-memory mutations:

```yaml
state:
  lookupKeyTemplate: "order-{orderId}"
  mutationKeyTemplate: "order-{orderId}"
  defaults:
    status: pending
  transitions:
    - onExample: confirm-order
      set:
        status: confirmed
    - onExample: cancel-order
      set:
        status: cancelled
```

This supports create-update-delete flows, sequence state, and other stateful patterns without a real database.

## Where Mock Data Lives In A Suite

```text
my-suite/
  api/       # contracts and OpenAPI/Protobuf schemas
  mock/      # mock metadata files and exchange examples
```

The suite hydration layer normalizes metadata paths and generates resolver and runtime URLs so the UI and runtime can reference them consistently.

!!! tip "Authoring guidance"
    - Keep large mock definitions in `mock/` rather than inline in `suite.star`.
    - Use named exchange examples instead of giant inline response bodies.
    - Define fallback behavior explicitly so the mock surface degrades predictably.
    - Use state transitions only when the suite genuinely needs request-driven stateful behavior.

## Related Pages

- [Mocking Reference](mocking-reference.md) — complete field reference for surfaces, operations, state, exchanges, and parameters
- [Suites](suites.md) — suite structure and how mocks fit into the topology
- [Suite Authoring Reference](suite-authoring.md) — mock folder layout and authoring patterns
