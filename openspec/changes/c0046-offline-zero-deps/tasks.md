---
slug: c0046-offline-zero-deps
title: Self-contained worker with miniredis fallback
---

## Task 1: Add miniredis fallback to worker config

Make `REDIS_URL` optional in the worker config. When empty, the worker logs a warning and uses miniredis.

- **File**: `apps/mework-mezon-worker/config.go`
- **Change**: Remove the requirement for `REDIS_URL`. Remove the fatal error if it's missing. The field stays but defaults to empty.
- **Test**: config_test.go (create or update) — verify that loading config without `REDIS_URL` succeeds and leaves `RedisURL` empty.

## Task 2: Wire miniredis fallback in worker main

Replace the `log.Fatal` when `REDIS_URL` is empty with a miniredis fallback. Log a clear warning.

- **File**: `apps/mework-mezon-worker/main.go`
- **Change**: In the Redis connection section, add an `else` branch that starts `miniredis.Run()`, creates a `redis.NewClient` pointing at it, and logs the warning.
- **New dependency**: `github.com/alicebob/miniredis/v2` (already transitive; may need explicit `require` in root `go.mod`)
- **Test**: Start worker without `REDIS_URL`, verify it connects and operates normally.

## Task 3: Update deployment docs

Document that Redis is optional for development/testing and required for production.

- **File**: `docs/deployment-guide.md`
- **Change**: Add a note explaining the miniredis fallback, the warning message, and when to use real Redis.
