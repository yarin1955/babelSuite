Returns Control Plane

Reverse-logistics suite that shows BabelSuite's richer mock runtime with split metadata, templating, constraints, fallback, state, and multi-protocol surfaces.

Structure

- `suite.star`: declarative topology
- `profiles/`: Launch profiles for local, canary, and peak-season refund traffic.
- `api/`: OpenAPI and protobuf contracts for returns and refund pricing.
- `mock/`: Mock payloads plus metadata that control dispatch, fallback, and state.
- `tasks/`: Bootstrap hooks for Kafka topics and refund routing tables.
- `tests/`: Smoke and manual-review verification flows for reverse logistics.
- `fixtures/`: Seeded return cases and customer profiles.
- `policies/`: Refund-limit and event-schema validation policies.
