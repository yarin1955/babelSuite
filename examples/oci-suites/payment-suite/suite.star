load("@babelsuite/postgres", "pg")
load("@babelsuite/kafka", "kafka")
load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")

# Pre-registered Starlark Modules return strict structs.
db = service.run()
kafka = service.run()
stripe_mock = service.mock(after=[db])
bootstrap_topics = task.run(file="bootstrap_topics.sh", image="bash:5.2", after=[kafka])
migrations = task.run(file="migrate.py", image="python:3.12", after=[db])
payment_gateway = service.run(after=[db, stripe_mock, migrations])
fraud_worker = service.run(after=[kafka, bootstrap_topics, payment_gateway])
checkout_baseline = traffic.baseline(plan="checkout_baseline.star", target="http://payment_gateway:8080", after=[payment_gateway, fraud_worker])
checkout_smoke = test.run(file="checkout_smoke.py", image="python:3.12", after=[checkout_baseline]).export("reports/junit.xml", name="checkout-test-report", on="always", format="junit").export("coverage/cobertura.xml", name="checkout-coverage", on="always", format="cobertura")
