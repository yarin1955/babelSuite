# Security Policy

## Supported Versions

Security fixes are applied to the latest release on the `main` branch. Older versions are not actively patched.

| Version | Supported |
|---------|-----------|
| Latest  | ✅ |
| Older   | ❌ |

## Reporting a Vulnerability

**Please do not file a public GitHub issue for security vulnerabilities.**

To report a security issue, email the maintainers directly or open a [private security advisory](https://github.com/babelsuite/babelsuite/security/advisories/new) on GitHub.

Include as much of the following as possible:

- Type of issue (e.g., authentication bypass, SSRF, injection, information disclosure)
- Full path to the affected source file(s)
- Reproduction steps or proof of concept
- Potential impact and severity assessment

We will acknowledge receipt within **48 hours** and aim to provide an initial assessment within **7 days**. Once a fix is confirmed, we will coordinate a disclosure timeline with you.

## Security Considerations for Deployment

- Set `JWT_SECRET` to a cryptographically random value of at least 32 bytes. Never use the default `change-me` value in any non-local environment.
- The catalog service validates registry URLs against a blocklist of private and reserved IP ranges to prevent SSRF. Do not disable this check unless the registry is genuinely local and `allowLocalNetwork: true` is explicitly set.
- Restrict the control plane API (port 8090) to trusted networks. It exposes administrative operations including platform settings, agent registration, and execution management.
- Remote agent shared secrets (`AGENT_SHARED_SECRET`) and mock shared secrets (`MOCK_SHARED_SECRET`) should be rotated regularly and kept out of source control.
