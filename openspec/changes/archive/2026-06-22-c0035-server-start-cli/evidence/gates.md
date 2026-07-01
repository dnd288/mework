# Gates — c0035-server-start-cli

Branch: `feat/c0035-server-start-cli` (from `main`)
Toolchain: go1.26.4 (satisfies go.mod `go 1.25.7`)

| Gate | Command | Result |
|------|---------|--------|
| Build (workspace) | `go build ./...` | PASS (exit 0) |
| Build (apps) | `go vet ./apps/...` | PASS (exit 0) |
| Vet | `make vet` (all libs/* modules) | PASS (exit 0) |
| Test | `make test` (`go test -p 1 ./...` per module) | PASS (exit 0) — DB-backed tests skip without `TEST_DATABASE_URL` |
| Import boundary | `libs/tools/import-guard` + `libs/shared/import_boundary_test.go` | PASS — no new `libs/client → libs/server` edge (seam keeps the hub dependency in `apps/mework`) |
| Spec validate | `openspec validate c0035-server-start-cli --strict` | PASS — "Change is valid" |

## TDD per unit

- **Unit 1 — injection seam.** RED: `cmd_server_test.go` failed to compile
  (`undefined: serverStartCmd`, `SetServerStarter`). GREEN: added
  `server_hook.go` (`ServerStarter` type, `serverStartFn` var, `SetServerStarter`),
  `cmd_server.go` (`server start --listen`, default behavior = no override),
  registered `serverCmd` under `groupRuntime` in `help.go`. Tests pass.
- **Unit 2 — in-process hub wiring.** `apps/mework/main.go` calls
  `cli.SetServerStarter(runHub)`; `runHub` lifts the boot sequence from
  `apps/mework-server/main.go` (LoadConfig → optional `--listen` override →
  RunMigrations → NewStore → NewServer → ListenAndServe + signal/context graceful
  shutdown) with the required driver blank-imports. `go mod tidy` left the root
  `go.mod` requires empty (workspace `use` directives supply local + transitive
  deps); the workspace build is green.

## Not covered

- Task 3.4 (live smoke: `mework server start` serving `/healthz` on `:8080`)
  requires a running Postgres + the key env vars and is left unchecked — out of
  scope for the unit/CI gates here. The seam, command behavior, default/override
  listen handling, and the "not available" path are unit-tested.
