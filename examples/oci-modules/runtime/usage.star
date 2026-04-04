load("@babelsuite/runtime", "container", "mock", "service", "script", "load", "scenario")

# Container entry points with after= preserved.
cache = container.run(
    name="redis-cache",
    image="redis:alpine",
    after=["migrate-db"],
    env={"ALLOW_EMPTY_PASSWORD": "yes"},
    ports={"6379": 6379},
    volumes={"./tmp/redis": "/data"},
    command=["redis-server", "--appendonly", "yes"],
)

prepared_api = container.create(
    name="payments-api",
    image="ghcr.io/babelsuite/payments-api:latest",
    after=["redis-cache"],
    env={"REDIS_ADDR": "redis-cache:6379"},
)

shared_proxy = container.get(name="otel-collector", after=["payments-api"])

# Container object methods for setup, debugging, and teardown.
prepared_api.copy(src="./fixtures/app.yaml", dest="/app/config/app.yaml")
probe = cache.exec(command=["redis-cli", "ping"])
recent_logs = cache.logs(tail=20)
cache_ip = cache.ip()
cache_port = cache.port(6379)
details = cache.inspect()
prepared_api.start()
prepared_api.stop(timeout=5)
prepared_api.delete(force=True)

# Native BabelSuite mocks come from the suite's mock/ folder.
orders_mock = mock.serve(
    name="orders-mock",
    source="./mock/orders",
    after=["payments-api"],
)

# Native mock object methods focus on BabelSuite behavior, state, and previewability.
orders_ready = orders_mock.wait_ready()
orders_url = orders_mock.url()
orders_logs = orders_mock.logs(tail=20)
orders_mock.reset_state()
orders_preview = orders_mock.preview(operation="get-order")

# External mock daemons belong to the service module, not the native mock module.
catalog_compat = service.prism(
    name="catalog-compat",
    spec_path="./compat/prism/openapi.yaml",
    port=4010,
    after=["orders-mock"],
)

legacy_compat = service.custom(
    name="legacy-compat",
    command=["node", "./scripts/custom_mock.js"],
    port=9090,
    after=["catalog-compat"],
)

# External service object methods cover daemon lifecycle and debug access.
catalog_compat.wait_ready(endpoint="/health", timeout=10)
catalog_url = catalog_compat.url()
catalog_logs = catalog_compat.logs(tail=20)
catalog_compat.stop()
legacy_compat.kill()

# Scripts are short-lived synchronous tasks that must finish before the suite continues.
# script.file(...) is the primary file-based task form; script.bash(...) stays as convenience sugar.
bootstrap = script.file(
    name="bootstrap",
    file_path="./scripts/bootstrap.sh",
    interpreter="bash",
    env={"APP_ENV": "local"},
    after=["legacy-compat"],
)

migrate = script.sql_migrate(
    name="migrate-db",
    target="db",
    sql_dir="./sql",
    after=["bootstrap"],
)

seed = script.exec(
    name="seed-data",
    command=["make", "seed"],
    cwd="./scripts",
    env={"SEED_PROFILE": "local"},
    after=["migrate-db"],
)
seed.assert_success()
seed_exit_code = seed.exit_code
seed_stdout = seed.stdout
seed_stderr = seed.stderr

# Load runs drive concurrency, throughput, staged ramps, and threshold budgets.
checkout_load = load.http(
    name="checkout-http-load",
    plan="./load/http_checkout.star",
    target="http://payments-api:8080",
    env={"ORDERS_URL": orders_url},
    after=["seed-data"],
    tags=["perf", "checkout"],
)
checkout_load.assert_success()
checkout_load_rps = checkout_load.rps_avg
checkout_load_p95 = checkout_load.latency.p95_ms
checkout_load_thresholds = checkout_load.thresholds
storefront_k6 = load.k6(
    name="storefront-k6",
    file_path="./load/storefront_k6.js",
    env={"BASE_URL": "http://payments-api:8080"},
    after=["checkout-http-load"],
)
storefront_k6.assert_success()

# Scenarios act as the attacker layer and compile the resulting test report.
smoke = scenario.go(
    name="checkout-smoke",
    test_dir="./scenarios/go",
    objectives=["checkout", "payments"],
    tags=["smoke", "ci"],
    env={"BASE_URL": "http://payments-api:8080", "ORDERS_URL": orders_url},
    after=["payments-api", "orders-mock", "catalog-compat", "legacy-compat", "seed-data", "checkout-http-load", "storefront-k6"],
)
smoke_passed = smoke.passed
smoke_exit_code = smoke.exit_code
smoke_duration_ms = smoke.duration_ms
smoke_logs = smoke.logs
smoke_summary = smoke.summary
smoke_failed = smoke.summary.failed
smoke_artifacts = smoke.artifacts_dir
