load("@babelsuite/postgres", "pg", "connect", "insert", "select", "delete", "upsert")
load("@babelsuite/runtime", "container")

db = pg(name="payments-db", database="payments")

db_ready = connect(db)

seed_merchant = insert(
    db,
    table="merchants",
    values={
        "merchant_id": "m-100",
        "status": "active",
    },
    after=["payments-db-connect"],
)

upsert_merchant = upsert(
    db,
    table="merchants",
    values={
        "merchant_id": "m-100",
        "status": "vip",
    },
    conflict_columns=["merchant_id"],
    after=["payments-db-insert-merchants"],
)

read_merchant = select(
    db,
    table="merchants",
    columns=["merchant_id", "status"],
    where={"merchant_id": "m-100"},
    after=["payments-db-upsert-merchants"],
)

api = container.run(
    name="payments-api",
    image="ghcr.io/acme/payments-api:latest",
    env={
        "DATABASE_URL": db["url"],
    },
    after=["payments-db", "payments-db-select-merchants"],
)

delete_merchant = delete(
    db,
    table="merchants",
    where={"merchant_id": "m-100"},
    after=["payments-api"],
)
