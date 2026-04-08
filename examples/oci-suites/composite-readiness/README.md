Composite Readiness

Minimal workspace suite that composes another suite through a dependency manifest and adds one final smoke step.

Structure

- `suite.star`: declarative topology with nested suite composition.
- `dependencies.yaml`: alias-to-reference manifest for imported suites, including version, profile, and input wiring.
- `dependencies.lock.yaml`: exact resolved artifact pins for imported suites.
- `profiles/`: profile metadata for launching the composite suite.
- `scenarios/go/`: smoke test assets for the final readiness validation step.
