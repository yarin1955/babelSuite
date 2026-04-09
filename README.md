# babelSuite

An open-source, container-native orchestrator for complex test suites and multi-language simulators, powered by Starlark.

## Documentation

The repository now includes a real documentation site in [docs/index.md](docs/index.md), built with a docs site config at [mkdocs.yml](mkdocs.yml).

Start here:

- [Getting Started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [Control Plane](docs/control-plane.md)
- [Configuration](docs/configuration.md)
- [Platform Settings](docs/platform.md)
- [Catalog](docs/catalog.md)
- [Suites](docs/suites.md)
- [Suite Authoring](docs/suite-authoring.md)
- [Dependency Manifests](docs/dependencies.md)
- [Profiles](docs/profiles.md)
- [Profile Runtime](docs/profile-runtime.md)
- [Modules](docs/modules.md)
- [Runtime Library](docs/runtime-library.md)
- [Mocking](docs/mocking.md)
- [Mocking Reference](docs/mocking-reference.md)
- [Execution](docs/execution.md)
- [Agents](docs/agents.md)
- [Environments](docs/environments.md)
- [Authentication](docs/auth.md)
- [API](docs/api.md)
- [CLI](docs/cli.md)
- [Examples](docs/examples.md)
- [Development](docs/development.md)
- [Operations](docs/operations.md)

### Run Docs Locally

Install the docs dependencies:

```powershell
pip install -r docs/requirements.txt
```

Then start the local docs server:

```powershell
mkdocs serve
```

The documentation site will be available at `http://127.0.0.1:8000/`.
