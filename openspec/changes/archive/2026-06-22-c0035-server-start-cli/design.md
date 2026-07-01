## Context

The hub is a separate binary; we want `mework server start` in-process for a one-binary local
UX. The constraint is the module split: `libs/client` (the CLI) must not import `libs/server`
(it would pull pgx/chi/goose into the client). The repo already solves this kind of problem
with a package-var injection seam (`runner.SetSessionResolverFactory`).

## Goals / Non-Goals

**Goals:**
- `mework server start` runs the hub in-process with the same config as `mework-server`.
- No `libs/server` import in the `libs/client` module.

**Non-Goals:**
- Removing or changing `apps/mework-server` (kept for container/prod deploys).
- A zero-dependency / embedded-DB dev mode (the hub stays Postgres-only).
- Changing hub config or routes.

## Decisions

- **Injection seam, wired in `apps/mework`.** `libs/client/cli` declares
  `type ServerStarter func(ctx context.Context, listen string) error`, a package var
  `serverStartFn ServerStarter`, and `func SetServerStarter(ServerStarter)`. The
  `server start` command calls `serverStartFn`; nil → friendly error. `apps/mework/main.go`
  injects a closure that does `hub.LoadConfig` (with `--listen` override) → migrations →
  pool → `hub.NewServer` → `http.Server.ListenAndServe` with signal-based graceful shutdown
  (lift from `apps/mework-server/main.go`). Root `go.mod` adds the `libs/server` requires.
- **Config from env, `--listen` override.** Reuse `hub.LoadConfig`; if `--listen` is set,
  override `cfg.ListenAddr`. Surface a clear error when required env (`DATABASE_URL`,
  `SERVER_KEY`, `MEWORK_SECRET_KEY`) is missing — `LoadConfig` already validates.
- **Keep `mework-server`.** The standalone binary remains the deploy artifact; `server start`
  is the dev/example convenience sharing the exact same wiring.

## Risks / Trade-offs

- [Client binary size] → `apps/mework` now links `libs/server`, growing the `mework` binary.
  Acceptable: it's the all-in-one dev binary; `libs/client` as a library stays lean.
- [Two wirings drift] → factor the shared boot into one helper if `apps/mework-server` and the
  injected closure diverge; for now both call the same `hub`/`store` funcs.
- [Import-guard] → verify no `libs/client → libs/server` edge is introduced; the seam keeps
  the dependency in `apps/mework` (root module) only.

## Migration Plan

Additive. New command + seam + app wiring. Existing `mework-server` deploys are unaffected.
