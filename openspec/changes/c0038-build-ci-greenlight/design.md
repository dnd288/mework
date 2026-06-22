## Context

The repo was restructured into a `go.work` workspace: binaries moved to
`apps/mework` and `apps/mework-server`; component code moved under
`libs/{shared,server,client,sandbox,tests,tools}`. The `Makefile` was not
updated. Current state, verified by running the targets:

- `make build` → `stat /d/mework/client/cmd/mework: directory not found`
  (target uses `./client/cmd/mework`, `./server/cmd/mework-server`).
- `make build-server` / `build-shared` / etc. → `cd: can't cd to server`
  (targets use `cd server`, `cd shared`, … instead of `cd libs/<module>`).
- `go.mod` declares `go 1.25.7`; `go.work` declares `go 1.26`. CI's `setup-go`
  uses `go-version-file: go.mod`, i.e. 1.25.7 — which cannot build the
  `go 1.26` workspace without an implicit toolchain download.
- `make test` iterates `MODULES := libs/shared libs/server libs/client
  libs/sandbox libs/tests libs/tools` (these paths are already correct), but
  the suite is red for two reasons: (a) `libs/tests/e2e` is a design-only
  harness whose `World` verbs `panic("design-only")`, so those scenarios crash;
  (b) two `libs/tests/integration` tests destroy their own fixtures mid-test.

The `apps/...` packages build cleanly via `go build ./apps/...`, so the
production code is fine — the breakage is entirely in build tooling and tests.

## Goals / Non-Goals

**Goals:**
- `make build`, `make build-all`, `make install`, and every `make
  build-*`/`test-*` target succeed from a clean checkout, resolving to the real
  `apps/` + `libs/` paths.
- `go.mod`, `go.work`, and CI agree on a Go toolchain that builds the workspace.
- `make test` (the no-DB default / ship path) is green across every module, with
  the design-only `e2e` package excluded from the default run (and still
  compilable with `-tags e2e`).

**Non-Goals:**
- Fixing the DB-backed `libs/tests/integration` behavioral failures (deferred —
  see D4).
- Building the `libs/tests/e2e` `World` harness (the `panic("design-only")`
  verbs). That is a separate, larger change.
- Removing the legacy `/api/v1/jobs/claim` route / SSE-push migration.
- Any production runtime code change, new dependency, or schema change.

## Decisions

### D1 — Repoint Makefile targets to the workspace layout

Update the binary targets to `./apps/mework` and `./apps/mework-server`, and the
per-module targets to `cd libs/<module>`. Keep `BINARY`/`SERVER_BINARY` names
and `LDFLAGS` as-is. `install` switches from `$(CMD)`/`$(SERVER_CMD)` to the
`apps/...` paths. This is mechanical path correction; no target semantics change.

_Alternative considered:_ delete the per-module `cd <module>` targets and drive
everything from the root with `go build ./...`. Rejected — the
`project-structure` spec requires per-module targets so each module's suite runs
independently (and CI's `module-build-and-test` matrix depends on them).

### D2 — Reconcile the Go toolchain by bumping `go.mod` to match `go.work`

Set `go.mod`'s `go` directive to `1.26` to match `go.work` (the installed
toolchain is 1.26.x and CI reads `go-version-file: go.mod`). This makes the
single source CI already consumes correct for the whole workspace.

_Alternatives considered:_ (a) lower `go.work` to `1.25.7` — rejected, the
workspace and installed toolchain are already on 1.26 and downgrading risks
unbuilding code that uses 1.26 features; (b) pin CI to a hardcoded version
string instead of `go-version-file` — rejected, keeps the version in two places
and re-introduces skew. Bumping `go.mod` keeps one source of truth.

### D3 — Exclude the design-only e2e package from the default test run via a build tag

Add a `//go:build e2e` constraint to the `libs/tests/e2e` package files so the
default `go test ./...` (and therefore `make test`) does not compile/run the
design-only scenarios that `panic("design-only")`. The package can still be run
deliberately with `go test -tags e2e ./e2e/...` once the harness is built. This
keeps the scaffolding in-tree (documented intent) without making `make test`/CI
red, and the future "build the e2e harness" change simply drops the tag.

_Alternatives considered:_ (a) delete the e2e files — rejected, they encode the
acceptance scenarios we intend to wire; (b) convert every verb to `t.Skip`
instead of `panic` — rejected, more churn than a single build tag and still
compiles dead scaffolding into the default run; (c) drop `libs/tests` from
`MODULES` — rejected, that also drops the working `integration` suite.

### D4 — Defer the DB-backed integration failures (scope correction)

Initially these read as pure test-isolation bugs (a 401 from a `DELETE FROM
runtimes` in an `act` step that wiped the just-minted `rt_token`; a 409 from a
reused runtime `code`). Implementing those fixes was attempted and reverted: the
underlying subtests then fail on **deeper behavioral assertions** that probe
not-yet-wired agent-hub behavior —

- `message_bus_test.go` (publish→SSE→ack) expects a dispatch event on
  `runner.<id>.dispatch` after a webhook; the current poll/queue model enqueues a
  job rather than pushing to that topic, so it times out (target-state, like the
  `claim-route returns 404` subtest).
- `pipeline_test.go` self-retrigger depends on shared bot-identity state recorded
  by the prior subtest's write-back; a unique runtime code alone changed the
  `full_flow` webhook result (`200` vs `202`), exposing self-retrigger/dispatch
  semantics that need real investigation.
- `TestChannelRouting_E2E` is the H4 tenant-scoping gap.

These are **out of scope** for a build/CI greenlight and are tracked separately
(channel tenant scoping + async → `c0040`; the SSE-push / claim-route migration
and a dedicated integration-test behavioral pass → their own changes). This
change leaves `libs/tests/integration` untouched. Those tests run only with
`TEST_DATABASE_URL`, so they gate neither `make build` nor the no-DB
`make test`/ship path.

## Risks / Trade-offs

- **[Hiding the e2e suite with a build tag lets it rot]** → Mitigation: the
  proposal explicitly records the follow-up change to build the harness and
  remove the tag; the scenarios remain in-tree and reviewable.
- **[`go 1.26` bump excludes contributors on older toolchains]** → Mitigation:
  the workspace already requires 1.26; this only makes the existing requirement
  explicit and consistent. Document the minimum in the build docs if not already.
- **[Repointed paths still subtly wrong]** → Mitigation: verification runs the
  actual targets (`make build`, `make build-all`, `make test-all`) in CI; the
  delta-spec scenarios assert zero-exit with no path errors.

## Migration Plan

No deploy/runtime migration — this is build tooling and tests. Steps: (1) bump
`go.mod` toolchain; (2) repoint Makefile targets; (3) tag the e2e package; (4)
fix the two integration tests; (5) run `make build`, `make vet`, `make test`
locally and confirm green; (6) confirm CI green. Rollback is a plain git revert
with no state implications.

## Open Questions

- None blocking. If the team prefers to keep `go.mod` at 1.25.x for downstream
  consumers, D2 flips to lowering `go.work` instead — to be confirmed at review,
  but the recommended path is bumping `go.mod` to 1.26.
