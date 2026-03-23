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