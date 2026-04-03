load("@babelsuite/runtime", "container", "mock", "script", "scenario")

claims_mock = mock(name="claims-mock")
seed_reference_data = script(name="seed-reference-data", after=["claims-mock"])
claims_bridge = container(name="claims-bridge", after=["claims-mock", "seed-reference-data"])
claims_smoke = scenario(name="claims-smoke", after=["claims-bridge"])
