load("@babelsuite/kafka", "kafka", "create_topic", "delete_topic", "set_group_offset")
load("@babelsuite/runtime", "container")

broker = kafka(name="broker")

payments_topic = create_topic(
    broker,
    topic="payments.events",
    partitions=3,
    replication_factor=1,
)

replay_offsets = set_group_offset(
    broker,
    group="fraud-worker",
    topic="payments.events",
    partition=0,
    offset=12,
    after=["broker-create-topic-payments-events"],
)

consumer = container.run(
    name="payments-consumer",
    image="ghcr.io/acme/payments-consumer:latest",
    env={
        "KAFKA_BOOTSTRAP_SERVERS": broker["bootstrap_servers"],
    },
    after=["broker", "broker-offset-fraud-worker-payments-events"],
)

cleanup_topic = delete_topic(broker, topic="payments.events", after=["payments-consumer"])
