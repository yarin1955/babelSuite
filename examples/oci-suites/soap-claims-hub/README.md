SOAP Claims Hub

Legacy claims sandbox with a SOAP intake surface, XML envelopes, and APISIX-fronted dispatch into the BabelSuite mock engine.

Structure

- `suite.star`: declarative topology
- `profiles/`: SOAP endpoint runtime knobs and partner header defaults.
- `api/`: WSDL contract published to legacy claim submitters.
- `mock/`: Schema-driven SOAP mock definitions that render XML envelopes at runtime.
- `scripts/`: Bootstrap hooks for claim code tables and partner fixtures.
- `scenarios/`: Smoke coverage for submit and lookup SOAP exchanges.
- `fixtures/`: Seeded partner claims and policy fixtures.
- `policies/`: SOAP fault and envelope validation policies.
