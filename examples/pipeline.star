# BabelSuite Pipeline Example
def run_simulation(env):
    if env == "ci":
        return [
            {"name": "simulators-python", "image": "python:3.10-alpine"},
            {"name": "test-suite", "image": "golang:1.20-alpine"}
        ]
    return [
        {"name": "simulators-local", "image": "python:3.10-alpine"}
    ]

# The global 'pipeline' variable will be parsed by BabelSuite
pipeline = run_simulation("ci")
