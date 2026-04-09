---
title: Development
---

# Development

[Back to index](index.md)

## Main Processes

During local development you usually run:

- backend server
- frontend dev server
- optional remote worker
- optional docs server

## Backend

From `backend/`:

```powershell
go run ./cmd/server
```

Optional worker:

```powershell
go run ./cmd/agent
```

## Frontend

From `frontend/`:

```powershell
npm install
npm run dev
```

## Docs

From the repo root:

```powershell
pip install -r docs/requirements.txt
mkdocs serve
```

## Useful Backend Commands

Run tests:

```powershell
go test ./...
```

Sync example content:

```powershell
go run ./cmd/sync-examples
```

Seed the local registry:

```powershell
go run ./cmd/seed-zot
```

## CLI

Build or run the CLI from:

- `backend/cmd/ctl`

Example:

```powershell
go run ./cmd/ctl -- version
```

## Frontend Quality Checks

From `frontend/`:

```powershell
npm run typecheck
npm run build
```

## Docs Deployment

The repository now includes:

- `.github/workflows/docs.yml`

That workflow installs the docs dependencies and deploys the generated site.
