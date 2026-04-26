# Contributing to BabelSuite

Thank you for taking the time to contribute. This document covers how to report issues, propose changes, and submit pull requests.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Reporting Bugs](#reporting-bugs)
- [Requesting Features](#requesting-features)
- [Development Setup](#development-setup)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Commit Messages](#commit-messages)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

All participants are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md). Please read it before engaging with the community.

## Reporting Bugs

Before opening a bug report, search [existing issues](https://github.com/babelsuite/babelsuite/issues) to see if it has already been reported.

When filing a new bug:

1. Use the **[Bug Report](.github/ISSUE_TEMPLATE/bug_report.yml)** template.
2. Include the BabelSuite version, operating system, and runtime environment.
3. Provide a minimal, reproducible example if possible.
4. Attach relevant logs from the control plane (`backend/` server output) or the browser console.

For security vulnerabilities, do **not** file a public issue. See [SECURITY.md](SECURITY.md) instead.

## Requesting Features

Feature requests are welcome. Please open a **[Feature Request](.github/ISSUE_TEMPLATE/feature_request.yml)** and describe:

- The problem you are trying to solve.
- Your proposed solution or API shape.
- Alternatives you have considered.

Large changes should be discussed in a [GitHub Discussion](https://github.com/babelsuite/babelsuite/discussions) or a design issue before significant implementation work begins.

## Development Setup

### Prerequisites

| Tool | Purpose |
|------|---------|
| Go 1.22+ | Backend control plane and CLI |
| Node.js 20+ | Frontend dev server |
| Docker | Local container execution |
| MongoDB or PostgreSQL | Primary datastore |
| Redis (optional) | Cache layer |

### Steps

```bash
# 1. Fork and clone
git clone https://github.com/<your-fork>/babelsuite.git
cd babelsuite

# 2. Copy the environment file and adjust as needed
# The repo root .env has sensible local defaults

# 3. Start the control plane
cd backend
go run ./cmd/server

# 4. Start the frontend (separate terminal)
cd frontend
npm install
npm run dev
```

The UI is available at `http://localhost:5173`.  
Default credentials: `admin@babelsuite.test` / `admin`.

### Seeding Local Registry Data

```bash
cd backend
go run ./cmd/seed-zot
```

### Running Tests

```bash
# Backend unit and integration tests
cd backend
go test ./...

# Frontend type checking
cd frontend
npm run typecheck
```

## Submitting a Pull Request

1. Fork the repository and create a branch from `main`:
   ```bash
   git checkout -b fix/my-fix
   ```
2. Make your changes, following the code style of the surrounding files.
3. Add or update tests as appropriate.
4. Ensure all tests pass locally.
5. Open a PR against the `main` branch and fill in the [pull request template](.github/PULL_REQUEST_TEMPLATE.md).

Pull requests are reviewed by maintainers. Small, focused PRs are easier to review and merge than large sweeping changes.

## Commit Messages

Use the conventional format:

```
<type>: <short summary>

<optional body>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.

Examples:
- `feat: add support for Kubernetes node selectors`
- `fix: resolve SSE disconnect on reconnect with Last-Event-ID`
- `docs: expand mocking reference for state transitions`

## Testing

- **Backend** — use `go test ./...` from `backend/`. Integration tests that require MongoDB or Docker are gated by build tags.
- **Frontend** — `npm run typecheck` checks TypeScript types. Component tests live alongside source files.
- New features require test coverage. Bug fixes should include a regression test where practical.

## Documentation

Documentation lives in `docs/` and is built with [MkDocs Material](https://squidfunk.github.io/mkdocs-material/).

```bash
pip install -r docs/requirements.txt
mkdocs serve
```

If your change adds or modifies a user-facing behaviour, update the relevant doc page. New concepts need an entry in `docs/index.md` and a link from the navigation in `mkdocs.yml`.
