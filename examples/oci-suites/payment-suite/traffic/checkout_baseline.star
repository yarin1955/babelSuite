load("@babelsuite/runtime", "traffic")

shopper = traffic.user(
    name="shopper",
    weight=1,
    wait=traffic.between(1, 3),
    tasks=[
        traffic.task(
            name="create-charge",
            request=traffic.post(
                "/charges",
                name="create-charge",
                json={
                    "merchantId": "merchant-001",
                    "amount": 4200,
                    "currency": "USD",
                    "cardToken": "tok_visa",
                },
            ),
            checks=[
                traffic.threshold("status", "==", 200),
                traffic.threshold("latency.p95_ms", "<", 600, sampler="create-charge"),
            ],
        ),
        traffic.task(
            name="get-payment",
            request=traffic.get("/payments/payment-123", name="get-payment"),
            checks=[
                traffic.threshold("status", "==", 200),
                traffic.threshold("latency.p95_ms", "<", 400, sampler="get-payment"),
            ],
        ),
    ],
)

traffic.plan(
    users=[shopper],
    shape=traffic.stages([
        traffic.stage(duration="3s", users=4, spawn_rate=2),
        traffic.stage(duration="5s", users=8, spawn_rate=4),
        traffic.stage(duration="6s", users=12, spawn_rate=6),
        traffic.stage(duration="2s", users=0, spawn_rate=6, stop=True),
    ]),
    thresholds=[
        traffic.threshold("http.error_rate", "<", 0.01),
        traffic.threshold("http.p95_ms", "<", 600, sampler="create-charge"),
        traffic.threshold("http.p95_ms", "<", 400, sampler="get-payment"),
    ],
)
