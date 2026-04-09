Identity Broker

OIDC and SAML integration sandbox with deterministic login failures, cert rotation, and secret injection paths.

Structure

- `suite.star`: declarative topology
- `profiles/`: Realm, issuer, and session-storage overrides by environment.
- `api/`: OIDC bridge OpenAPI and SAML mapping definitions.
- `mock/`: OIDC JWKS payloads and SAML assertion fixtures.
- `tasks/`: Realm seeders and certificate bootstrap helpers.
- `tests/`: Login smoke suites and expired-session validation.
- `fixtures/`: Realm definitions and claim payloads.
- `policies/`: Cookie scope and token issuance validation.
