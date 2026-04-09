load("@babelsuite/runtime", "test", "suite")

payments = suite.run(ref="payments-module")
readiness_smoke = test.run(file="go/readiness_smoke_test.go", image="golang:1.24", after=[payments])
