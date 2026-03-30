Payment Suite

Bank-grade reference environment with Postgres, Kafka, Wiremock, and a full fraud worker topology.

Structure

- `suite.star`: declarative topology
- `profiles/`: Environment variable toggles and runtime overrides.
- `api/`: Immutable OpenAPI and protobuf contracts for the suite.
- `mock/`: Wiremock mappings and scenario-specific stub bodies.
- `scripts/`: Boot-time migrations and broker preparation scripts.
- `scenarios/`: Smoke tests and attack-path executions.
- `fixtures/`: Static input data for cards, merchants, and seeded accounts.
- `policies/`: Rego payload validation and ledger invariants.
