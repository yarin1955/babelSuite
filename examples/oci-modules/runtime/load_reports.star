load("@babelsuite/runtime", "load")

http_run = load.http(
    name="checkout-http-load",
    plan="./load/http_checkout.star",
    target="http://payments-api:8080",
    env={"ORDERS_URL": "http://orders-mock:8080"},
    tags=["perf", "checkout"],
)
http_run.assert_success()
http_passed = http_run.passed
http_exit_code = http_run.exit_code
http_duration_ms = http_run.duration_ms
http_rps = http_run.rps_avg
http_peak = http_run.rps_peak
http_p95 = http_run.latency.p95_ms
http_thresholds = http_run.thresholds
http_samplers = http_run.summary_by_sampler
http_artifacts = http_run.artifacts_dir

grpc_run = load.grpc(
    name="orders-grpc-load",
    plan="./load/grpc_orders.star",
    target="dns:///orders-api:9090",
    after=["checkout-http-load"],
)
grpc_run.assert_success()

legacy_run = load.locust(
    name="legacy-locust",
    file_path="./load/legacy_locust.py",
    host="http://legacy-api:8080",
    after=["orders-grpc-load"],
)
legacy_run.assert_success()

graphql_run = load.jmx(
    name="graphql-jmx",
    plan_path="./load/graphql.jmx",
    properties={"threads": "10", "ramp": "2"},
    after=["legacy-locust"],
)
graphql_run.assert_success()

k6_run = load.k6(
    name="storefront-k6",
    file_path="./load/storefront_k6.js",
    env={"BASE_URL": "http://payments-api:8080"},
    after=["graphql-jmx"],
)
k6_run.assert_success()
