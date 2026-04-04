load("@babelsuite/postgres", "pg")
load("@babelsuite/kafka", "kafka")
load("@babelsuite/runtime", "container", "mock", "script", "scenario")

# Pre-registered Starlark Modules return strict structs.
db = container.run(name="db")
kafka = container.run(name="kafka")
stripe_mock = mock(name="stripe-mock", after=["db"])
bootstrap_topics = script(name="bootstrap-topics", after=["kafka"])
migrations = script(name="migrations", after=["db"])
payment_gateway = container.run(name="payment-gateway", after=["db", "stripe-mock", "migrations"])
fraud_worker = container.run(name="fraud-worker", after=["kafka", "bootstrap-topics", "payment-gateway"])
checkout_smoke = scenario(name="checkout-smoke", after=["payment-gateway", "fraud-worker"])
