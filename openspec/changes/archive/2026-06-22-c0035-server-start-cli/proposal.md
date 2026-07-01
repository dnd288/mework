## Why

Bringing the system up today requires running a **separate `mework-server` binary**
(`apps/mework-server`) alongside the `mework` CLI. For local development and the
`examples/remote-claude` walkthrough we want a single ergonomic command:
**`mework server start`** runs the hub in-process, so the three components come up as three
plain commands — `mework server start`, `mework daemon start`, `mework sandbox start -w .`.

The hub itself already factors cleanly: `hub.LoadConfig` reads the environment, `store.
RunMigrations` + `store.NewStore` prepare Postgres, and `hub.NewServer` returns an
`http.Handler`. `apps/mework-server/main.go` just wires those together. This change exposes
the same wiring behind a CLI command without coupling the `libs/client` module to
`libs/server`.

## What Changes

- **`mework server start`** (new command, Runner group). Boots the hub in-process: load
  config from the environment (same vars as `mework-server`: `DATABASE_URL`, `SERVER_KEY`,
  `MEWORK_SECRET_KEY`, optional `LISTEN_ADDR`/`MELLO_BASE_URL`/…), run migrations, open the
  pool, serve `hub.NewServer` with graceful shutdown. `--listen` overrides the address.
- **Injection seam keeps `libs/client` server-free.** `libs/client/cli` exposes
  `SetServerStarter(fn)` + an unexported `serverStartFn` var; the `server start` command
  calls it. If no starter is wired, the command exits with a clear "server start is not
  available in this build" error. This mirrors the existing
  `runner.SetSessionResolverFactory` seam (`libs/client/runner/session_dispatch.go:52`).
- **`apps/mework` wires the real starter.** `apps/mework/main.go` calls
  `cli.SetServerStarter(...)` with a closure that imports `libs/server/hub` + `libs/server/
  platform/store` (the body lifted from `apps/mework-server/main.go`). `apps/*` are in the
  **root** module, which may import `libs/server`; the `libs/client` module is untouched.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cli`: adds a `server start` command (Runner group) that runs the hub in-process, reading
  the same configuration as the standalone server.

## Impact

- **Client code:** `libs/client/cli/cmd_server.go` (new command), `libs/client/cli/
  server_hook.go` (new `SetServerStarter` seam), `help.go` (register under `groupRuntime`).
- **App wiring:** `apps/mework/main.go` (call `SetServerStarter`); root `go.mod` gains
  `mework/libs/server` requires. `apps/mework-server` stays as-is (still buildable).
- **Reuses** `hub.LoadConfig`, `hub.NewServer`, `store.RunMigrations`, `store.NewStore`.
- **Docs:** note `mework server start` in the deployment/example docs (still needs Postgres
  + the key env vars). Because it reads all config from the environment, it doubles as the
  **command of a docker-compose service** (a `mework` image running `server start`) alongside
  a Postgres service — the example can ship a compose file using it.
- **Module boundary:** the import-guard (`libs/tools/import-guard`,
  `libs/shared/import_boundary_test.go`) must still pass — `libs/client` gains no
  `libs/server` import.
- Independent of c0036/c0037; first of the three for the streamlined local UX.
