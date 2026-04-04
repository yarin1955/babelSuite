@babelsuite/runtime

Core suite runtime module exposing container.run/create/get, mock.serve, service.wiremock/prism/custom, plus script and scenario entry points for topology orchestration.

Details

- Repository: `localhost:5000/babelsuite/runtime`
- Version: `0.9.0`
- Tags: `0.9.0`, `0.8.3`, `latest`
- Pull: `babelctl run localhost:5000/babelsuite/runtime:0.9.0`
- Fork: `babelctl fork localhost:5000/babelsuite/runtime:0.9.0 ./stdlib-runtime`

Usage

See `usage.star`, `suite_example.star`, `scripts/container_lifecycle.star`, `scripts/mock_lifecycle.star`, `scripts/service_lifecycle.star`, `scripts/script_results.star`, and `scenario_reports.star` for the recommended runtime patterns.

Runtime Examples

- `usage.star`: quick overview of `container.*`, native `mock.*`, external `service.*`, `script.*`, and `scenario.*` entry points plus object methods.
- `suite_example.star`: declarative topology that keeps `suite.star` easy to read.
- `scripts/container_lifecycle.star`: imperative container operations such as `exec`, `copy`, `logs`, `inspect`, and teardown.
- `scripts/mock_lifecycle.star`: native mock operations such as `wait_ready`, `url`, `logs`, `reset_state`, and `preview`.
- `scripts/service_lifecycle.star`: external compatibility service operations such as `wait_ready`, `url`, `logs`, `stop`, and `kill`.
- `scripts/script_results.star`: synchronous script execution with `exit_code`, `stdout`, `stderr`, and `assert_success()`.
- `scenario_reports.star`: Go, Python, and HTTP scenario execution with `passed`, `exit_code`, `duration_ms`, `logs`, `summary`, and `artifacts_dir`.
- `mock/`: sample BabelSuite-native mock assets.
- `compat/`: sample WireMock and Prism compatibility assets.
- `scripts/` and `sql/`: sample assets for `script.file`, the `script.bash` convenience sugar, and `script.sql_migrate`.
- `scenarios/`: sample Go, pytest, and Hurl-style scenario assets.
