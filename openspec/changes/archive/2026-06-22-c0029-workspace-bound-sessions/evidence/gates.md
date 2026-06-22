# Gates — c0029-workspace-bound-sessions

## Gates run

| Gate | Result |
|------|--------|
| `go build ./...` | exit 0 |
| `make vet` | exit 0 |
| `go test -p 1 -coverprofile` (all modules) | exit 0, all green |
| `go tool cover -func` | total 22.1% |
| `openspec validate c0029-workspace-bound-sessions --strict` | exit 0, valid |

## Coverage

total: 22.1% of statements (merged across all 7 modules: root + libs/{shared,server,client,sandbox,tests,tools})

## Per-task commits

| Task | Commit |
|------|--------|
| 01 | a7bc20e |
| 02 | 2052749 |
| 03 | c714c60 |
| 04 | 6ff2588 |
| 05 | 163b9da |
| 06 | e34dab8 |

## Repair count

0

## Governing skills

- test-driven-development
- incremental-implementation
- code-simplification
- debugging-and-error-recovery
- code-review-and-quality
- security-and-hardening
- git-workflow-and-versioning
- documentation-and-adrs
