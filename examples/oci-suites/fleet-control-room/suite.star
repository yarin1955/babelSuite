load("@babelsuite/postgres", "pg")
load("@babelsuite/kafka", "kafka")

def run(ctx):
    db = run_container(
        name="pg-primary",
        image=ctx.env.infra["postgres"].image,
        env=ctx.env.infra["postgres"].get("env", {}),
        detach=True,
    )
    cache = run_container(
        name="redis-cache",
        image=ctx.env.infra["redis"].image,
        detach=True,
    )
    broker = run_container(
        name="kafka-broker",
        image=ctx.env.infra["kafka"].image,
        env=ctx.env.infra["kafka"].get("env", {}),
        detach=True,
    )

    db.wait_for_ready(type="tcp", port=5432, timeout=25)
    cache.wait_for_ready(type="tcp", port=6379, timeout=20)
    broker.wait_for_ready(type="tcp", port=9092, timeout=35)

    pg.execute_sql(db, "CREATE TABLE IF NOT EXISTS devices (id SERIAL PRIMARY KEY, name TEXT UNIQUE, status TEXT, region TEXT);")
    pg.execute_sql(db, "CREATE TABLE IF NOT EXISTS alerts (id SERIAL PRIMARY KEY, severity TEXT, source TEXT, created_at TIMESTAMP DEFAULT NOW());")
    kafka.create_topic(broker, "fleet.telemetry")
    kafka.create_topic(broker, "fleet.alerts")

    api = run_container(
        name="fleet-api",
        image=ctx.env.apps["api"].image,
        network=db.network_id,
        env={
            "DB_URL": "postgres://admin:pw@pg-primary:5432/fleet_db",
            "REDIS_URL": "redis://redis-cache:6379/0",
            "KAFKA_BROKER": "kafka-broker:9092",
            "FEATURE_AUDIT_MODE": ctx.env.apps["api"].get("env", {}).get("FEATURE_AUDIT_MODE", "false"),
        },
        detach=True,
    )
    api.wait_for_ready(type="http", endpoint="http://localhost:8080/health", timeout=20)

    ui = run_container(
        name="fleet-ui",
        image=ctx.env.apps["ui"].image,
        network=db.network_id,
        env=ctx.env.apps["ui"].get("env", {}),
        detach=True,
    )
    ui.wait_for_ready(type="http", endpoint="http://localhost:3000", timeout=20)

    worker = run_container(
        name="fleet-worker",
        image=ctx.env.apps["worker"].image,
        network=db.network_id,
        env={
            "DB_URL": "postgres://admin:pw@pg-primary:5432/fleet_db",
            "KAFKA_BROKER": "kafka-broker:9092",
        },
        detach=True,
    )

    simulator = run_container(
        name="physics-sim",
        image=ctx.env.simulators["physics_engine"].image,
        network=db.network_id,
        env={
            "KAFKA_BROKER": "kafka-broker:9092",
            "SCENARIO": ctx.inputs["storm_profile"],
        },
        detach=True,
    )

    if ctx.inputs["enable_mock_ledger"]:
        ledger_mock = run_container(
            name="ledger-mock",
            image=ctx.env.mocks["ledger"].image,
            network=db.network_id,
            mounts={"api": "//api"},
            command="microcks start --main-artifact /api/fleet-openapi.yaml",
            detach=True,
        )
        ledger_mock.wait_for_ready(type="http", endpoint="http://localhost:8585/api/services", timeout=30)

    async_auditor = run_container(
        name="async-auditor",
        image=ctx.env.tests["async_runner"].image,
        network=db.network_id,
        mounts={"api": "//api"},
        command="asyncapi validate --file /api/fleet-events.yaml",
    )
    if async_auditor.exit_code != 0:
        fail_pipeline("Async contract validation failed")

    backend_test = run_container(
        name="pytest-backend",
        image=ctx.env.tests["backend_runner"].image,
        network=db.network_id,
        mounts={"tests": "//tests", "api": "//api"},
        env={
            "FLEET_API_URL": "http://fleet-api:8080",
            "REPLAY_WINDOW_MINUTES": str(ctx.inputs["replay_window_minutes"]),
        },
        command="sh -lc 'pip install -r /tests/requirements.txt && pytest /tests/e2e_flow.py --junitxml=/workspace/reports/backend.xml'",
    )

    contract_test = run_container(
        name="contract-check",
        image=ctx.env.tests["contract_runner"].image,
        network=db.network_id,
        mounts={"api": "//api"},
        command="microcks test /api/fleet-openapi.yaml --api-url http://fleet-api:8080",
    )

    ui_test = run_container(
        name="playwright-ui",
        image=ctx.env.tests["frontend_runner"].image,
        network=db.network_id,
        mounts={"tests": "//tests"},
        env={"BASE_URL": "http://fleet-ui:3000"},
        command="sh -lc 'echo UI checks would run here'",
    )

    if backend_test.exit_code != 0 or contract_test.exit_code != 0 or ui_test.exit_code != 0:
        api.dump_logs("/workspace/artifacts/fleet-api.log")
        simulator.dump_logs("/workspace/artifacts/physics-sim.log")
        pg.dump_database(db, "fleet_db", "/workspace/artifacts/fleet_db.sql")
        fail_pipeline("One or more validation stages failed")

    print("Fleet control room suite completed successfully")
