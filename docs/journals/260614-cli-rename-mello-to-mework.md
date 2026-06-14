# Renaming CLI Identity from Mello to Mework (TDD)

**Date**: 2026-06-14 13:45
**Severity**: Medium
**Component**: CLI, Build Tooling
**Status**: Resolved

## What Happened

We successfully renamed the CLI's self-identity from `mello` to `mework` while maintaining compatibility with the target Kanban API.
- **Bucket A (CLI self-identity)** was fully renamed:
  - CLI binary renamed from `mello` to `mework` (build target in Makefile and `.goreleaser.yml` updated).
  - CLI command directory moved from `cmd/mello/` to `cmd/mework/`.
  - Config directory renamed from `~/.mello` to `~/.mework` in path helpers (`MeworkDir()`).
  - CLI-own behavior env vars renamed: `MELLO_HOME` -> `MEWORK_HOME`, `MELLO_PROFILE` -> `MEWORK_PROFILE`, and `MELLO_DEBUG` -> `MEWORK_DEBUG`.
- **Bucket B (External Mello Product)** was preserved intact:
  - External API URLs remain `mello.mezon.vn`.
  - Personal access token prefix remains `mello_pat_`.
  - Connection env vars remain `MELLO_BASE_URL`, `MELLO_WORKSPACE_ID`, and `MELLO_API_KEY`.
  - Package imports (`internal/mello/`) and DB schema columns (`account_boards.mello_board_id`) were preserved.

The renaming followed a strict TDD workflow:
- **Phase 01**: Lock identity in tests (RED). We updated `config_test.go`, `lifecycle_test.go`, and `trigger_test.go` to assert `~/.mework` paths and use `MEWORK_HOME`, and added a new assertion `rootCmd.Use == "mework"` in `help_test.go`.
- **Phase 02**: Rename internal CLI identity (GREEN). We updated `internal/cli/paths.go`, `config.go`, and `cmd/mello/main.go` until the tests passed.
- **Phase 03**: Move command directory and build config. We moved `cmd/mello/` to `cmd/mework/`, updated the Makefile and `.goreleaser.yml`, and deleted the stale compiled `mello.exe` from the repo root.
- **Phase 04**: Sweep documentation and final verification. We updated commands and paths in `README.md` and user/developer guides, and verified with grep that zero Bucket A leftovers remained.

## The Brutal Truth

Leaving the CLI binary and local files named `mello` while the Go module path was `mework` and the backend was `mework-server` was a confusing, half-baked mess. The name collision between the integration CLI itself and the third-party product (Mello) was a landmine. A blind find-and-replace would have quietly broken the external API integration, token validation, and the database schema. Manual, line-by-line triage of hundreds of matches was tedious but necessary. TDD was the only thing preventing us from breaking the daemon lifecycle tests, which previously set `MELLO_HOME` and would have leaked to the real `~/.mework` directory once the path helpers switched over.

## Technical Details

During Phase 01, we modified the assertions in `internal/cli/config_test.go`:
```go
// Line 6
os.Setenv("MEWORK_HOME", tempDir)
// Line 34
want := filepath.Join(home, ".mework")
```
When run initially, the test suite failed:
```
--- FAIL: TestMelloDir (0.00s)
    config_test.go:12: MelloDir() = "/Users/username/.mello", want "/Users/username/.mework"
```
And compiled errors occurred in daemon tests because `MeworkDir` did not exist yet:
```
internal/daemon/lifecycle_test.go:25: undefined: cli.MelloDir
```

After Phase 03 and 04, the binary builds cleanly via `make build`:
```bash
go build -ldflags "..." -o bin/mework ./cmd/mework
go build -ldflags "..." -o bin/mework-server ./cmd/mework-server
```
And all tests compile and pass green:
```
ok  	mework/cmd/mework	(cached)
ok  	mework/internal/cli	(cached)
ok  	mework/internal/daemon	(cached)
```

## What We Tried

- **Blind find-and-replace**: Rejected immediately. This would have corrupted `internal/mello/` imports, the `mello_pat_` token prefix, and the database migration column `mello_board_id`, breaking external API communication.
- **Config directory migration shim**: We discussed adding code to automatically migrate `~/.mello` to `~/.mework` if it existed. We rejected this because the CLI has no active production users yet; adding migration code would be premature optimization and unnecessary technical debt (YAGNI).

## Root Cause Analysis

The root cause was a failure to separate the CLI's own brand identity from the integration target during initial scaffolding. We coupled the local CLI binary name and config structures to the external service ("Mello"), which created severe namespace pollution when the project pivoted to its own name ("Mework").

## Lessons Learned

1. **Brand-isolate integrations early**: Do not name your local CLI binary or config paths after a third-party API it integrates with. Keep them under your own brand namespaces from day one.
2. **Use TDD to guide refactoring**: Writing/updating test assertions to lock down what should change (and what should not change) before touching production code is the only way to perform mechanical renames safely.
3. **Clean up build artifacts**: Stale binaries (like the committed `mello.exe`) must be removed from the repo as part of the rename phase to prevent developers from executing obsolete builds.

## Next Steps

- **Verify goreleaser pipeline**: Run a snapshot release dry-run to ensure artifact packaging works.
- **Verify installation**: Ensure onboarding docs do not reference the old `~/.mello` directory.
