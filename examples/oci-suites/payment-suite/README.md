Payment Suite

Bank-grade reference environment with Postgres, Kafka, Wiremock, and a full fraud worker topology.

Structure

- `suite.star`: declarative topology
- `profiles/`: Environment variable toggles and runtime overrides.
- `api/`: Immutable OpenAPI and protobuf contracts for the suite.
- `mock/`: Wiremock mappings and scenario-specific stub bodies.
- `tasks/`: Boot-time migrations and broker preparation jobs.
- `traffic/`: Native traffic plans used by semantic traffic profiles.
- `tests/`: Smoke tests and attack-path executions.
- `fixtures/`: Static input data for cards, merchants, and seeded accounts.
- `policies/`: Rego payload validation and ledger invariants.

Native Load Example

```python
checkout_baseline = traffic.baseline(
    plan="checkout_baseline.star",
    target="http://payment_gateway:8080",
    after=[payment_gateway, fraud_worker],
)
```

The checked-in plan is in `traffic/checkout_baseline.star`.

To switch the suite to a different native traffic style, keep the same plan shape and change only the runtime entrypoint:

- `traffic.smoke`: tiny preflight run
- `traffic.baseline`: normal expected load
- `traffic.stress`: push beyond capacity
- `traffic.spike`: sudden user jump
- `traffic.soak`: long endurance run
- `traffic.scalability`: expansion-focused run
- `traffic.step`: discrete incremental ramps
- `traffic.wave`: oscillating high/low cycles
- `traffic.staged`: several named phases
- `traffic.constant_throughput`: fixed request-rate cap
- `traffic.constant_pacing`: fixed interval per user
- `traffic.open_model`: fixed arrival-rate workload
