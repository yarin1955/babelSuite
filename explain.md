my-payment-suite/
├── babel.yaml              # The Master Manifest (Metadata & strict inputs)
├── suite.star              # The Brain (Starlark orchestration execution)
├── topology.yaml           # The Map (Binds logical names to Docker images)
│
├── profiles/               # The Environments (Variable overrides)
│   ├── default.yaml        
│   └── ci-heavy.yaml             
│
├── api/                    # The Simulators (Contracts for native Go mocks)
│   ├── stripe-v1.yaml      # OpenAPI (REST)
│   ├── ledger.proto        # Protobuf (gRPC)
│   └── events.yaml         # AsyncAPI (For the auditor container)
│
└── tests/                  # The Execution (Mounted scripts)
    ├── e2e_flow.py         # PyTest script (runs inside a generic Python container)
    └── requirements.txt    # Dependencies for the test


def run_async_audit(ctx):
    # 1. Boot the real message broker
    kafka = run_container(image="confluentinc/cp-kafka:7.4.0")
    
    # 2. Boot your real application that publishes events to Kafka
    app = run_container(
        image="my-go-publisher:latest",
        env={"KAFKA_URL": kafka.url}
    )
    
    # 3. BOOT THE AUDITOR! 
    # This container contains an open-source AsyncAPI validation CLI.
    # It connects to Kafka, listens to the topic, and checks every message 
    # against the contract mounted from your OCI package.
    auditor = run_container(
        name="async-police",
        image="asyncapi/validator:latest",
        command="listen --broker kafka:9092 --topic user-events --schema //api/async-contract.yaml",
        network=kafka.network_id
    )
    
    # If the app publishes a bad message, the Auditor container crashes with Exit Code 1.
    # BabelSuite catches the 1 and fails the pipeline!
    if auditor.exit_code != 0:
        fail("Application published an event that violated the AsyncAPI contract!")

def run_contract_suite(ctx):
    # 1. Boot the real database
    db = run_container(image="postgres:15")
    
    # 2. Boot the real API being tested
    api = run_container(image="my-go-api:latest", network=db.network_id)
    
    # 3. Boot MICROCKS as the test runner!
    contract_test = run_container(
        name="microcks-tester",
        image="microcks/microcks-cli:latest",
        network=db.network_id,
        command="microcks-cli test 'api-openapi.yaml' --api-url http://my-go-api:8080"
    )
    
    if contract_test.exit_code != 0:
        fail_pipeline("API broke the OpenAPI contract!")

def run_drone_suite(ctx):
    # 1. Boot the environment
    broker = run_container(image="kafka")
    sim = run_container(image="wind-simulator", network=broker.network_id)
    
    # 2. Boot the "Test Director" container. 
    # THIS is where your complex tests actually live.
    tester = run_container(
        name="pytest-director",
        image="registry/drone-tests:latest", # Contains all the complex Python logic
        network=broker.network_id,
        command="pytest /tests/complex_wind_scenarios.py"
    )
    
    if tester.exit_code != 0:
        fail_pipeline("The complex tests inside the container failed!")

# Load standard BabelSuite modules to abstract away the bash/exec commands
load("@babelsuite/kafka", "kafka")
load("@babelsuite/postgres", "pg")

def run_fleet_suite(ctx):
    print("Executing topology: " + ctx.env.name)

    # ---------------------------------------------------------
    # 1. CORE INFRASTRUCTURE LAYER
    # ---------------------------------------------------------
    db = run_container(
        name="pg-primary",
        image=ctx.env.infra["postgres"].image,
        env={"POSTGRES_USER": "admin", "POSTGRES_PASSWORD": "pw"},
        detach=True
    )
    broker = run_container(
        name="kafka-broker",
        image=ctx.env.infra["kafka"].image,
        detach=True
    )

    # Block until TCP ports are accepting connections
    db.wait_for_ready(timeout=20, type="tcp", port=5432)
    broker.wait_for_ready(timeout=30, type="tcp", port=9092)

    # ---------------------------------------------------------
    # 2. STATE INITIALIZATION
    # ---------------------------------------------------------
    # Use the loaded helpers to run .exec() commands cleanly
    pg.execute_sql(db, "CREATE DATABASE fleet_db;")
    kafka.create_topic(broker, "telemetry_data")
    kafka.create_topic(broker, "fleet_alerts")

    # ---------------------------------------------------------
    # 3. APPLICATION LAYER
    # ---------------------------------------------------------
    api = run_container(
        name="fleet-api",
        image=ctx.env.apps["api"].image,
        network=db.network_id, # Link all subsequent containers to this network
        env={
            "DB_URL": "postgres://admin:pw@pg-primary:5432/fleet_db",
            "KAFKA_BROKER": "kafka-broker:9092"
        },
        detach=True
    )
    api.wait_for_ready(timeout=15, type="http", endpoint="http://localhost:8080/health")

    # Run database migrations using the API container's built-in CLI tool
    migration = api.exec(["fleet-cli", "migrate", "up"])
    if migration.exit_code != 0:
        fail_pipeline("Database migration failed: " + migration.stderr)

    ui = run_container(
        name="fleet-ui",
        image=ctx.env.apps["ui"].image,
        network=db.network_id,
        env={"API_URL": "http://fleet-api:8080"},
        detach=True
    )
    ui.wait_for_ready(timeout=15, type="http", endpoint="http://localhost:3000")

    # ---------------------------------------------------------
    # 4. HARDWARE SIMULATION LAYER
    # ---------------------------------------------------------
    # Boot the simulator. It immediately starts pumping 1,000 GPS coords/sec into Kafka.
    physics_sim = run_container(
        name="drone-physics-sim",
        image=ctx.env.simulators["physics_engine"].image,
        network=db.network_id,
        env={"KAFKA_BROKER": "kafka-broker:9092", "SCENARIO": "heavy_storm"},
        detach=True
    )

    # ---------------------------------------------------------
    # 5. PARALLEL TEST EXECUTION
    # ---------------------------------------------------------
    print("Environment stable. Launching parallel test runners...")

    # Runner A: Python/PyTest checks the API and Database state
    backend_test = run_container(
        name="pytest-backend",
        image=ctx.env.tests["backend_runner"].image,
        network=db.network_id,
        command="pytest /tests/integration --junitxml=/workspace/reports/backend.xml",
        detach=True
    )

    # Runner B: Playwright checks the UI to ensure the "storm alerts" are rendering
    e2e_test = run_container(
        name="playwright-e2e",
        image=ctx.env.tests["frontend_runner"].image,
        network=db.network_id,
        command="npx playwright test --output=/workspace/reports/playwright",
        detach=True
    )

    # ---------------------------------------------------------
    # 6. SYNCHRONIZATION & ARTIFACT EXTRACTION
    # ---------------------------------------------------------
    # The script blocks here until both testing containers finish their processes
    backend_exit = backend_test.wait()
    e2e_exit = e2e_test.wait()

    if backend_exit == 0 and e2e_exit == 0:
        print("Suite Passed. All simulated telemetry processed correctly.")
    else:
        print("Suite Failed. Extracting diagnostic artifacts...")
        
        # Pull logs and DB state before the engine destroys the isolated network
        api.dump_logs("/workspace/artifacts/api_crash.log")
        physics_sim.dump_logs("/workspace/artifacts/sim_crash.log")
        pg.dump_database(db, "fleet_db", "/workspace/artifacts/db_dump.sql")
        
        fail_pipeline("One or more test suites failed. Check /workspace/artifacts.")