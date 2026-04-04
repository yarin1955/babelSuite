load("@babelsuite/runtime", "scenario")

go_suite = scenario.go(
    name="checkout-go",
    test_dir="./scenarios/go",
    objectives=["checkout", "payments"],
    tags=["smoke", "ci"],
    env={"BASE_URL": "http://payments-api:8080"},
)
go_passed = go_suite.passed
go_exit_code = go_suite.exit_code
go_duration_ms = go_suite.duration_ms
go_logs = go_suite.logs
go_summary = go_suite.summary
go_failed = go_suite.summary.failed
go_artifacts = go_suite.artifacts_dir

python_suite = scenario.python(
    name="checkout-python",
    test_dir="./scenarios/python",
    objectives=["checkout"],
    tags=["regression"],
    env={"BASE_URL": "http://payments-api:8080"},
    after=["checkout-go"],
)
python_passed = python_suite.passed
python_exit_code = python_suite.exit_code
python_duration_ms = python_suite.duration_ms
python_summary = python_suite.summary

http_suite = scenario.http(
    name="checkout-http",
    collection_path="./scenarios/http/checkout.hurl",
    objectives=["edge"],
    tags=["api"],
    env={"BASE_URL": "http://payments-api:8080"},
    after=["checkout-python"],
)
http_passed = http_suite.passed
http_exit_code = http_suite.exit_code
http_duration_ms = http_suite.duration_ms
http_logs = http_suite.logs
http_summary = http_suite.summary
