load("@babelsuite/runtime", "container", "mock", "service", "script", "load", "scenario")

redis = container.run(
    name="redis-cache",
    image="redis:alpine",
    ports={"6379": 6379},
)

api = container.run(
    name="payments-api",
    image="ghcr.io/acme/payments:latest",
    after=["redis-cache"],
    env={"REDIS_ADDR": "redis-cache:6379"},
)

orders_mock = mock.serve(
    name="orders-mock",
    source="./mock/orders",
    after=["payments-api"],
)

catalog_compat = service.prism(
    name="catalog-compat",
    spec_path="./compat/prism/openapi.yaml",
    port=4010,
    after=["orders-mock"],
)

seed = script.file(name="seed-data", file_path="./scripts/bootstrap.sh", interpreter="bash", after=["payments-api"])
migrate = script.sql_migrate(name="migrate-db", target="db", sql_dir="./sql", after=["seed-data"])
checkout_load = load.http(name="checkout-http-load", plan="./load/http_checkout.star", target="http://payments-api:8080", after=["migrate-db"])
storefront_k6 = load.k6(name="storefront-k6", file_path="./load/storefront_k6.js", after=["checkout-http-load"])
smoke = scenario.go(name="checkout-smoke", test_dir="./scenarios/go", objectives=["checkout"], tags=["smoke"], after=["storefront-k6", "catalog-compat"])
