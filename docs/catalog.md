---
title: Catalog
---

# Catalog

[Back to index](index.md)

## What The Catalog Does

The catalog is the registry-backed discovery surface for packages.

It combines:

- configured registries from platform settings
- workspace-known suites
- built-in example module metadata

## How Discovery Works

For each configured registry, the catalog service:

1. calls `GET /v2/_catalog`
2. enumerates repositories
3. calls `GET /v2/<repo>/tags/list`
4. builds a package view from the repository plus any locally known metadata

If a discovered repository matches a known workspace suite, the catalog can enrich the registry result with:

- suite title
- owner
- description
- module tags
- pull and fork commands
- inspectability

## Package Fields

A catalog package currently exposes:

- `id`
- `kind`
- `title`
- `repository`
- `owner`
- `provider`
- `version`
- `tags`
- `description`
- `modules`
- `status`
- `score`
- `pullCommand`
- `forkCommand`
- `inspectable`
- `starred`

## Favorites

The catalog also supports favorites:

- `GET /api/v1/catalog/favorites`
- `POST /api/v1/catalog/favorites/{packageId}`
- `DELETE /api/v1/catalog/favorites/{packageId}`

These let the frontend mark commonly used packages without changing the registry source itself.

## Frontend Route

The main browser is:

- `/catalog`

This page is the discovery layer for registry-backed content.

## Relationship To Suites

The catalog and the runnable suite inventory are related, but not identical:

- **Catalog** is what registries advertise
- **Suites** are what BabelSuite can currently inspect and run locally

That distinction is why a package can appear in the catalog even when it is not the current workspace source of truth for runnable suite details.
