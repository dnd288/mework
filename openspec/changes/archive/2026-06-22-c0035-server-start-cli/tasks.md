## 1. Injection seam (TDD)

- [x] 1.1 Write `libs/client/cli/cmd_server_test.go` (fail first): with a fake starter set via
      `SetServerStarter`, `server start --listen :9999` invokes it with `:9999`; with no
      starter wired, the command returns a clear "not available" error.
- [x] 1.2 Add `libs/client/cli/server_hook.go`: `ServerStarter` type, `serverStartFn` var,
      `SetServerStarter`.
- [x] 1.3 Add `libs/client/cli/cmd_server.go`: `server start` command (flag `--listen`,
      default `:8080`) calling `serverStartFn`; register under `groupRuntime` in `help.go`.

## 2. Wire the in-process hub in apps/mework

- [x] 2.1 In `apps/mework/main.go`, call `cli.SetServerStarter(...)` with a closure:
      `hub.LoadConfig` (override `ListenAddr` from `--listen`) → `store.RunMigrations` →
      `store.NewStore` → `hub.NewServer` → `http.Server` with graceful shutdown (lift from
      `apps/mework-server/main.go`).
- [x] 2.2 Add `mework/libs/server` (+ transitive) requires to the root `go.mod`; `go mod tidy`.

## 3. Validation

- [x] 3.1 `make vet` + `go build ./...` green; the `mework` binary builds with `server start`.
- [x] 3.2 Import-guard / `import_boundary_test.go` still passes (no `libs/client → libs/server`).
- [x] 3.3 `make test` green (new CLI test fails-first then passes).
- [ ] 3.4 Smoke: with Postgres + env set, `mework server start` serves `/healthz` on `:8080`.
