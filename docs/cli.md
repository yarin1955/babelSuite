---
title: CLI
---

# CLI

[Back to index](index.md)

## Binary

The CLI entrypoint is:

- `backend/cmd/ctl`

It builds and runs the `babelctl` command.

## Command Groups

The root command registry currently exposes these groups.

### Session

- `login`
- `logout`
- `whoami`

### Suites

- `catalog`
- `create`
- `suites`
- `profiles`

### Executions

- `runs`
- `run`
- `fork`

### Environments

- `environments`
- alias: `envs`

### System

- `version`

## Current Root Usage

- `catalog list | inspect <package>`
- `create <name> [destination]`
- `suites list | get <suite> | inspect <suite>`
- `profiles list <suite>`
- `runs list | get <id>`
- `run <suite|repository[:tag]> [--profile <profile.yaml>]`
- `fork <suite|repository[:tag]> [destination]`
- `environments list | reap <id> | reap-all`
- `version`

## Examples

```powershell
babelctl login
babelctl suites list
babelctl profiles list payment-suite
babelctl run localhost:5000/core-platform/payment-suite:workspace --profile local.yaml
babelctl environments list
```

## Scaffold Command

`create` is the suite template generator. It creates a starter suite layout on disk so users can begin with a valid package shape rather than building everything manually.
