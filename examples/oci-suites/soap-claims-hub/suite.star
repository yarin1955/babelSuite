load("@babelsuite/runtime", "service", "task", "test", "traffic", "suite")

claims_mock = service.mock()
seed_reference_data = task.run(file="seed_reference_data.sh", image="bash:5.2", after=[claims_mock])
claims_bridge = service.run(after=[claims_mock, seed_reference_data])
claims_smoke = test.run(file="claims_smoke.py", image="python:3.12", after=[claims_bridge])
