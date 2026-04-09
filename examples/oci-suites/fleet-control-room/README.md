Fleet Control Room

End-to-end vehicle orchestration environment with Redis, gRPC contracts, and mocked telemetry spikes.

Structure

- `suite.star`: declarative topology
- `profiles/`: Driver-specific runtime knobs for local, perf, and staging lanes.
- `api/`: gRPC protobuf definitions and REST gateway overlays.
- `mock/`: Telemetry playback feeds and fault injections for route spikes.
- `tasks/`: Redis seeders and topology bootstrap hooks.
- `tests/`: Control room smoke runs and degraded GPS scenarios.
- `fixtures/`: Vehicle manifests and fake GPS frames.
- `policies/`: Route SLA validation and forbidden-zone checks.
