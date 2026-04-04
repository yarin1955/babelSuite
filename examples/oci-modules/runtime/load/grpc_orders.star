load("@babelsuite/runtime", "load")

grpc_user = load.user(
    name="grpc-order-reader",
    wait=load.pacing(1),
    tasks=[
        load.task(
            name="get-order",
            request=load.grpc(
                method="orders.v1.OrderService/GetOrder",
                body={"orderId": "ord_123"},
                metadata={"x-suite-profile": "local"},
                name="orders.get-order",
            ),
            checks=[load.threshold("status", "==", 0)],
        ),
    ],
)

load.plan(
    users=[grpc_user],
    shape=load.stages([
        load.stage(duration="30s", users=5, spawn_rate=5),
        load.stage(duration="90s", users=25, spawn_rate=10),
    ]),
    thresholds=[load.threshold("grpc.p95_ms", "<", 250, sampler="orders.get-order")],
)
