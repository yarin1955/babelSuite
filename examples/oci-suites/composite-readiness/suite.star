load("@babelsuite/runtime", "scenario", "suite")

payments = suite.run(ref="payments-module")
readiness_smoke = scenario.go(name="readiness-smoke", test_dir="./scenarios/go", after=["payments"])
