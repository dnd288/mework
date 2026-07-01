# Verify gates — c0006-normalize-workflow-keyword

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | ✓ exit 0 |
| Vet | `go vet ./...` (make vet) | ✓ exit 0 |
| Test | `go test -p 1 ./...` (make test) | ✓ exit 0 — DB tests skipped (TEST_DATABASE_URL unset), not a failure |
| Coverage | `go tool cover -func` | total: 21.9% of statements |
| Spec | `openspec validate --strict` | ✓ valid |

- **Red status:** confirmed — `internal/server/webhook/parse_test.go` failed (undefined `NormalizeWorkflow`) before implementation.
- **Repairs:** 0
- **Skills applied:** test-driven-development, incremental-implementation, code-simplification, debugging-and-error-recovery, code-review-and-quality, security-and-hardening, git-workflow-and-versioning, documentation-and-adrs
