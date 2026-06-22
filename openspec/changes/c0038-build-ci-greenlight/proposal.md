## Why

The repository was restructured into a `go.work` workspace with binaries under
`apps/` and modules under `libs/`, but the build tooling was never updated to
match. The `Makefile` build/test targets still point at pre-restructure paths
(`./client/cmd/mework`, `./server/cmd/mework-server`, `cd shared`), so
`make build` and the per-module `make build-*`/`test-*` targets fail outright.
The Go toolchain is also skewed (`go.mod` declares `go 1.25.7` while `go.work`
requires `go 1.26`, and CI pins the version from `go.mod`), and two integration
tests destroy their own fixtures mid-test, turning a green suite red. The net
effect: `make build` is broken and `make test`/CI cannot go green — the project
is not buildable or releasable as checked in.

## What Changes

- Repoint every `Makefile` build/test/install target at the real workspace
  paths: binaries at `./apps/mework` and `./apps/mework-server`, modules under
  `libs/<name>` (so `build-shared` → `cd libs/shared`, etc.). `make build`,
  `make build-all`, and `make test-all` succeed.
- Resolve the Go toolchain skew so `go.mod`, `go.work`, and the CI
  `setup-go` version agree and the workspace builds with the pinned toolchain.
- Gate the `libs/tests/e2e` design-only harness (`panic("design-only")` verbs)
  behind a `//go:build e2e` tag so the default `go test ./...` / `make test` no
  longer compiles or runs the unbuilt scaffolding. The package still builds with
  `go test -tags e2e ./e2e/...`, and the future "build the e2e harness" change
  simply drops the tag.

**Scope correction (verified during implementation):** the DB-backed integration
failures first read as simple test-isolation bugs (a 401 from a self-deleted
runtime; a 409 from a reused runtime code). On attempting those fixes, the tests
proved to assert **deeper, not-yet-wired agent-hub behavior** — webhook→
`runner.<id>.dispatch` SSE push, self-retrigger/dispatch semantics, and
channel-routing tenant scoping — not mere fixtures. They are therefore **out of
scope here** and tracked as their own work (channel tenant scoping + async →
`c0040`; the SSE-push / claim-route migration and a dedicated integration-test
behavioral pass → separate changes). This change does **not** modify
`libs/tests/integration`. Those tests only run with `TEST_DATABASE_URL` set, so
they gate neither `make build` nor the no-DB `make test`/ship path.

Non-goals: building the e2e `World` harness; fixing the DB-backed integration
behavioral assertions; the `claim`-route-removal decision; rate limits /
artifact store / provider stubs — all tracked separately.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `project-structure`: the "independent module build and test" requirement
  currently asserts per-module build/test targets exist and run; the delta
  tightens it so those targets MUST resolve to the actual `apps/` + `libs/`
  workspace paths and succeed, and adds a requirement that the declared Go
  toolchain is consistent across `go.mod`, `go.work`, and CI.

## Impact

- **Build/ops:** `Makefile` (build/test/install/per-module targets), `go.mod`
  and/or `go.work` toolchain lines, `.github/workflows/ci.yml` (Go version
  resolution; possibly the e2e exclusion).
- **Tests:** `libs/tests/integration/message_bus_test.go`,
  `libs/tests/integration/pipeline_test.go`.
- **No production runtime code changes** (`apps/` and `libs/server|client|sandbox`
  packages are untouched); the fixes are confined to build tooling and tests.
- Unblocks every downstream contributor and the `/opsx:ship` verify gate, which
  runs `make vet`/`make test`.
