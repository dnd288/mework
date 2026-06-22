## Why

The production-readiness assessment flagged HTTP-layer and config hardening gaps that are
cheap to close and squarely within the implemented server:

- **No request-body cap** — the webhook handler `io.ReadAll`s the body before the 64 KB
  instruction cap, and there is no `MaxBytesReader`/global limit, so a large body is a
  memory-exhaustion vector (M5).
- **No slowloris protection** — the `http.Server` sets no `ReadHeaderTimeout`, so a client
  can hold connections open by trickling headers (M5).
- **`/healthz` couples liveness to the database and leaks the DB error** — a transient DB
  blip flaps liveness (causing needless restarts under an orchestrator), and the 503 body
  echoes the raw driver error (`"database unreachable: "+err`), a minor info disclosure (M7).
- **No minimum strength on the secret/HMAC keys** — `SERVER_KEY` / `MEWORK_SECRET_KEY` are
  accepted at any length (even 1 char) and silently SHA-256-stretched (L1).

These are SSE-safe to fix (the body cap and header timeout do not affect long-lived
`/jobs/subscribe` and `/sessions/{id}/stream` response streams).

## What Changes

- **Request-body cap.** Add `chimiddleware.RequestSize(4 MiB)` to the router so every request
  body is bounded (webhook payloads included); SSE response streams are unaffected.
- **Slowloris timeout.** Set `ReadHeaderTimeout: 10s` and `IdleTimeout: 120s` on the
  `http.Server` in both entrypoints (`apps/mework-server` and the in-process
  `apps/mework server start`). **No `WriteTimeout`** — SSE is long-lived; `IdleTimeout`
  exceeds the SSE heartbeat so kept-alive streams are not closed early.
- **Liveness/readiness split.** Add `GET /livez` (always 200 — process liveness, no DB
  dependency) and `GET /readyz` (DB ping → 200/503). `/readyz` and the retained `/healthz`
  return a **generic** `{"status":"not ready"}` body and **log** the underlying DB error
  server-side instead of leaking it.
- **Key strength.** `LoadConfig` rejects `SERVER_KEY` / `MEWORK_SECRET_KEY` shorter than 16
  characters with a clear error, in addition to the existing presence check.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `auth-and-secrets`: the "Required server secrets" requirement additionally enforces a
  **minimum key length** for `SERVER_KEY` and `MEWORK_SECRET_KEY` (fail fast on weak keys).
- `provider-gateway`: adds a **server HTTP hardening** requirement — bounded request bodies,
  a header-read timeout (without breaking SSE), and a liveness/readiness probe split that
  does not couple liveness to the database or leak internal errors.

## Impact

- **Server:** `libs/server/hub/config.go` (key length), `libs/server/hub/health.go`
  (`LivenessHandler`/`ReadinessHandler`, no error leak), `libs/server/hub/router.go`
  (`RequestSize` middleware + `/livez`/`/readyz` routes).
- **Apps:** `apps/mework-server/main.go` and `apps/mework/main.go` (`ReadHeaderTimeout`/
  `IdleTimeout`).
- **Tests:** `libs/server/hub/config_test.go` (new), `libs/server/hub/health_test.go`
  (liveness/readiness).
- Backward-compatible: `/healthz` is retained (now non-leaking). Existing deploys that set
  ≥16-char keys are unaffected; weak-key deploys fail fast with a clear message.
- No schema migration. Does not touch the SSE handlers or the message bus.
