# Fleet Control Room

This example shows the OCI suite package shape described in `explain.md` as a
real directory tree:

- `babel.yaml` holds package metadata and input definitions.
- `suite.star` describes orchestration.
- `topology.yaml` binds logical services to OCI images.
- `profiles/` overrides the base topology and inputs by environment.
- `api/` contains REST, gRPC, and async contracts.
- `tests/` contains mounted test code and dependencies.

The package models a fleet control environment with:

- real infrastructure containers
- an API and UI
- a ledger mock
- contract validation
- event auditing
- mounted Python tests
