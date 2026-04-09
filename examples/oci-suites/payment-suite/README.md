Payment Suite

Bank-grade reference environment with Postgres, Kafka, Wiremock, and profiles ready for settings-managed secret injection.

Structure

- `suite.star`: declarative topology
- `api/`: Contracts and schemas shipped with the suite.
- `fixtures/`: Static seed data and sample payloads.
- `mock/`: Mock schemas, metadata, and compatibility assets.
- `policies/`: Policy rules and invariants enforced by the suite.
- `profiles/`: Environment-specific runtime overrides and launch profiles.
- `tasks/`: Short-lived setup, seed, and migration jobs.
- `tests/`: Verification, smoke, regression, and browser assertions.
- `traffic/`: Native traffic plans and protocol-safe workload definitions.
