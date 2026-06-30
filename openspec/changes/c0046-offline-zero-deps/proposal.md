---
slug: c0046-offline-zero-deps
title: Self-contained worker with miniredis fallback
---

## Why

The current worker (`mework-mezon-worker`) and offline daemon both require external infrastructure that is impractical for a CLI tool running on a developer's machine:

- **Redis** is mandatory for the turbo engine (WebSocket state, message dedup, channel cursors). A developer must install Redis or run a Docker container before they can test any Mezon integration.

- **Redis** is mandatory for the turbo engine (WebSocket state, message dedup, channel cursors). A developer must install Redis or run a Docker container before they can test any Mezon integration.
- **Postgres** is required for the server mode. While acceptable for production deployments, it's heavy for local testing — the developer needs Docker running just to try the tool.

This creates a friction barrier: "install Docker, start Postgres, start Redis, then run." For a CLI tool meant to be run on a developer's workstation with `brew install mework`, requiring two databases is a non-starter.

## What Changes

- **miniredis fallback in the worker**: When `REDIS_URL` is not set, the worker starts an embedded in-memory Redis-compatible server using `miniredis` (already a transitive dep of the turbo SDK). Zero install, zero config, zero Docker. The turbo engine's state (dedup, cursors, activity) is ephemeral and lost on restart — acceptable for development/testing.
- **Worker can run without any external service**: The worker plus miniredis is the only process needed to connect Mezon bots and bridge messages to the server API. The server still needs Postgres, but the worker itself is self-contained.
- **No SQLite for now**: Adding SQLite as a Postgres alternative in the server would require maintaining two query layers and duplicating Postgres-specific features (`FOR UPDATE SKIP LOCKED`, advisory locks, pgx transactions). Not worth the complexity. The server stays Postgres-only; for local testing, `make test-db` starts Postgres in Docker.

### Breaking changes

- **None**. Adding a miniredis fallback is fully backward-compatible. Existing deployments with `REDIS_URL` continue to work unchanged.

## Capabilities

### New Capabilities
- `miniredis-fallback`: worker falls back to embedded miniredis when Redis is unavailable, enabling zero-dependency local operation

### Modified Capabilities
- `mezon-worker`: the worker no longer requires Redis. The `REDIS_URL` env var becomes optional. Logs a warning when falling back to in-memory state.

## Impact

- **Modified file**: `apps/mework-mezon-worker/main.go` — add miniredis fallback when `REDIS_URL` is empty
- **Modified file**: `apps/mework-mezon-worker/config.go` — `REDIS_URL` becomes optional, default to empty
- **Dependencies**: `miniredis` is already a transitive dep of the turbo SDK (`github.com/alicebob/miniredis/v2`). No new dependencies.
- **Documentation**: Update deployment guide to document that Redis is optional for the worker (production only).
