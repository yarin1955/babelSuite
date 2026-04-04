load("@babelsuite/runtime", "load")

shopper = load.user(
    name="shopper",
    weight=4,
    wait=load.between(1, 3),
    on_start=[
        load.post("/login", name="login"),
    ],
    tasks=[
        load.task(
            name="browse-catalog",
            weight=3,
            request=load.get("/catalog", name="catalog"),
            checks=[load.threshold("status", "==", 200)],
        ),
        load.task(
            name="checkout",
            weight=2,
            request=load.post("/checkout", name="checkout", json={"sku": "sku-123", "qty": 1}),
            checks=[
                load.threshold("status", "==", 200),
                load.threshold("latency.p95_ms", "<", 500, sampler="checkout"),
            ],
        ),
    ],
)

load.plan(
    users=[shopper],
    shape=load.stages([
        load.stage(duration="1m", users=10, spawn_rate=5),
        load.stage(duration="3m", users=50, spawn_rate=10),
        load.stage(duration="5m", users=100, spawn_rate=20),
        load.stage(duration="6m", users=0, spawn_rate=20, stop=True),
    ]),
    defaults={"headers": {"Content-Type": "application/json"}},
    data=[load.csv("./load/users.csv")],
    thresholds=[
        load.threshold("http.error_rate", "<", 0.01),
        load.threshold("http.p95_ms", "<", 500, sampler="checkout"),
    ],
)
