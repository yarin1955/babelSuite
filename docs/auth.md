---
title: Authentication
---

# Authentication

[Back to index](index.md)

## Supported Login Modes

BabelSuite currently supports:

- local email and password authentication
- direct OIDC single sign-on

The frontend reads auth configuration from the backend, so sign-in pages can hide or show local login and SSO options dynamically.

## Local Auth

The auth handler supports:

- sign up
- sign in
- current session lookup

The control plane seeds the initial admin account from:

- `ADMIN_EMAIL`
- `ADMIN_PASSWORD`

Local auth can be toggled with:

- `AUTH_PASSWORD_LOGIN_ENABLED`
- `AUTH_SIGNUP_ENABLED`

## OIDC

The current OIDC flow is:

- single provider
- direct browser login
- PKCE-enabled
- state cookie protected
- local JWT issued after callback
- group claim mapping for admin elevation

Important environment variables include:

- `OIDC_ENABLED`
- `OIDC_PROVIDER_ID`
- `OIDC_PROVIDER_NAME`
- `OIDC_ISSUER_URL`
- `OIDC_CLIENT_ID`
- `OIDC_CLIENT_SECRET`
- `OIDC_REDIRECT_URL`
- `OIDC_FRONTEND_CALLBACK_URL`
- `OIDC_SCOPES`
- `OIDC_PKCE_ENABLED`
- `OIDC_EMAIL_CLAIM`
- `OIDC_NAME_CLAIM`
- `OIDC_GROUPS_CLAIM`
- `OIDC_ADMIN_GROUPS`
- `AUTH_STATE_SECRET`

## Session Model

After successful local auth or OIDC login, the backend issues a local JWT for the frontend session.

Protected routes use middleware that:

- verifies the token
- populates session context
- supports query-token access where streaming endpoints need it

## Frontend Auth Routes

- `/sign-in`
- `/sign-up`
- `/forgot-password`
- `/auth/callback`

## Auth API Endpoints

Public:

- `GET /api/v1/auth/config`
- `POST /api/v1/auth/sign-up`
- `POST /api/v1/auth/sign-in`
- `GET /api/v1/auth/sso/providers`
- `GET /api/v1/auth/oidc/login`
- `GET /api/v1/auth/oidc/callback`

Protected:

- `GET /api/v1/auth/me`

Legacy short paths under `/auth/*` are also registered.
