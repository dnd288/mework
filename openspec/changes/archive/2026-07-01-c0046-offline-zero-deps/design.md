---
slug: c0046-offline-zero-deps
title: Self-contained worker with miniredis fallback
---

## Context

The turbo engine requires a `redis.Cmdable` for state management (message dedup, channel cursors, activity tracking, tier promotion). Currently the worker panics if `REDIS_URL` is not set. This design adds an embedded fallback using `miniredis` — an in-memory Redis implementation in pure Go.

## Decisions

### D1: miniredis, not a custom in-memory store

**Decision**: Use `miniredis` as the fallback when `REDIS_URL` is empty, rather than implementing a custom in-memory `redis.Cmdable`.

**Rationale**:
- `miniredis` is already a transitive dependency of the turbo SDK (`github.com/alicebob/miniredis/v2` in `mezon-go-sdk-turbo`'s go.mod)
- It implements the full `redis.Cmdable` interface — the exact interface the turbo engine expects
- Zero implementation cost: start a miniredis server, create a redis.Client pointing at it, pass to the engine
- No behavioral differences: the engine's state code works identically with real Redis or miniredis

**Alternatives considered**:
- Custom `redis.Cmdable` implementation — rejected because it would need to implement ~100+ methods to satisfy the interface, with high risk of subtle behavioral mismatches
- Skip the turbo engine entirely when no Redis — rejected because the engine is the core of the worker
- Require Redis always — rejected because it blocks local development

### D2: Ephemeral state is acceptable for dev

**Decision**: The miniredis fallback loses all state on worker restart. This is the expected trade-off for zero-install local development.

**Rationale**:
- Message dedup state is ephemeral — a few duplicate deliveries on restart are acceptable
- Channel cursors are lost — the worker re-learns channels from inbound messages
- Activity/tier state is rebuilt within seconds as the engine rebalances
- Production deployments use real Redis (persistent, restart-safe)

## Architecture

### Worker startup flow (updated)

```
Worker start
  │
  ├── Load config (MEZON_CONFIG / env)
  │
  ├── Redis available? (REDIS_URL set?)
  │   ├── Yes → connect to real Redis
  │   └── No  → start miniredis in-memory server
  │               └── log "WARNING: using in-memory state, lost on restart"
  │
  ├── Create turbo engine with redis.Cmdable (either real or miniredis)
  ├── Register bots
  ├── Start outbound poller
  └── engine.Run() ← blocks
```

### Config change

```
REDIS_URL → optional (default: empty = use miniredis)
```

When `REDIS_URL` is empty, the worker logs:
```
WARNING: REDIS_URL not set — using embedded in-memory Redis (state lost on restart)
For production, set REDIS_URL=redis://...
```

## Risks

- **[R1] Memory growth**: miniredis stores all state in-process memory. With thousands of bots and millions of messages, this could grow unbounded. Mitigation: the turbo engine's `StateTTL` (default 30 days) and `DedupCap` (2048 entries per bot) bound the state size. For development workloads (a handful of bots), memory is negligible.
- **[R2] Production safety**: A user might accidentally run without Redis in production and lose state on restart. Mitigation: the warning log message is prominent. We could add a `MEWORK_ENV=production` guard in the future that fails if Redis is not configured.
