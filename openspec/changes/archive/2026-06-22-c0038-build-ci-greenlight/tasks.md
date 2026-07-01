## 1. Go toolchain reconciliation

- [x] 1.1 Set the `go` directive in `go.mod` to `1.26` to match `go.work` (D2).
- [x] 1.2 Confirm `go.work` (`go 1.26`) and CI `setup-go` (`go-version-file: go.mod`) now resolve a consistent toolchain; no other version literals need changing.
- [x] 1.3 Verify `go build ./...` and `go vet ./...` succeed across the workspace with the pinned toolchain.

## 2. Makefile path corrections

- [x] 2.1 Repoint `build-mework` to `./apps/mework` and `build-mework-server` to `./apps/mework-server` (and update `CMD`/`SERVER_CMD` used by `install` accordingly).
- [x] 2.2 Repoint per-module targets `build-shared`/`build-server`/`build-client`/`build-sandbox` and `test-*` from `cd <module>` to `cd libs/<module>`.
- [x] 2.3 Run `make build`, `make build-all`, and `make test-all`; confirm each exits zero with no "directory not found" / "can't cd" errors.

## 3. Exclude design-only e2e package from default test run

- [x] 3.1 Add a `//go:build e2e` build constraint to every file in `libs/tests/e2e` (including `harness_test.go`, `api_test.go`, `bdd_test.go`, and the `NN_*_test.go` files) so the default build skips the design-only harness (D3).
- [x] 3.2 Confirm `go test ./...` in `libs/tests` no longer compiles/runs the `e2e` package, and that `go test -tags e2e ./e2e/...` still compiles it (for the future harness change).

## 4. Integration test behavioral failures — deferred (scope correction)

- [x] 4.1 Investigated the DB-backed integration failures: they assert not-yet-wired
      agent-hub behavior (webhook→`runner.<id>.dispatch` SSE push, self-retrigger/dispatch
      semantics, channel tenant routing), not mere fixtures. **Out of scope here**;
      `libs/tests/integration` is left untouched and the work is tracked separately
      (channel → `c0040`; SSE-push / claim-route + integration behavioral pass → their own
      changes). These tests run only with `TEST_DATABASE_URL`, so they gate neither
      `make build` nor the no-DB `make test`/ship path.

## 5. Verification

- [x] 5.1 `make build` + `make build-all` succeed (both binaries from `apps/`); `make vet`
      and `make test` (no DB — the ship/CI default path) are green; the `e2e` package is
      excluded by default and still compiles with `-tags e2e`.
- [x] 5.2 Run `openspec validate c0038-build-ci-greenlight --strict` and confirm it passes.
- [x] 5.3 Confirm the CI workflow (`.github/workflows/ci.yml`) build · vet · test and
      per-module jobs pass with the updated targets and toolchain.
