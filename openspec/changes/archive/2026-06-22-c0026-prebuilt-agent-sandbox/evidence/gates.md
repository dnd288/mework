# Gates — c0026-prebuilt-agent-sandbox

## Gates run

| Gate | Scope | Result |
|------|-------|--------|
| `go build ./...` | root module | pass |
| `make vet` (`go vet ./...`) | all 6 `libs/*` modules | pass |
| `make test -p 1` | all 6 `libs/*` modules (root `go test ./...` also run) | pass (every package `ok`) |
| `go tool cover -func` | merged profile across all modules | 20.4% total |
| `openspec validate c0026-prebuilt-agent-sandbox --strict` | change | pass |

## Coverage

**Total: 20.4%** of statements (merged coverage profile across all 7 modules via
`go tool cover -func`).

Note: the deterministic spec's single-module cover file
`/tmp/shipcode-c0026-prebuilt-agent-sandbox.cover` reports **0.0%** because the
repo is a multi-module Go workspace and root `go test ./...` only covers the apps
root module; the meaningful full-tree figure is **20.4%** from the merged profile.

`make test` ran all 6 `libs/*` modules with every package `ok`. DB-backed tests
were skipped (no `TEST_DATABASE_URL`), which is expected, not a failure.

## Per-task commits

| Task | Commit |
|------|--------|
| 01 | 73fa0d8 |
| 02 | ce68355 |
| 03 | a937758 |
| 04 | 8a2ff46 |
| 05 | 74df63c |

## Repair count

2

## Governing skills

- test-driven-development
- incremental-implementation
- code-simplification
- debugging-and-error-recovery
- code-review-and-quality
- security-and-hardening
- git-workflow-and-versioning
- documentation-and-adrs
