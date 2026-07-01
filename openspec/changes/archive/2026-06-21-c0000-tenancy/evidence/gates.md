# Ship Gates — c0000-tenancy

## Gates run

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS |
| Vet | `make vet` (`go vet ./...`) | PASS |
| Test | `go test -p 1 -coverprofile ./...` | PASS (DB-backed tests skip without `TEST_DATABASE_URL`) |
| Coverage | `go tool cover -func` | total **21.3%** (see `coverage.txt`) |
| Spec validate | `openspec validate c0000-tenancy --strict` | PASS — `Change 'c0000-tenancy' is valid` |

Toolchain: go.mod pins `go 1.25.7`; gates re-activate a Go >= 1.25 toolchain (`GOTOOLCHAIN=go1.25.7+auto`).

## Coverage

- Total: **21.3%** of statements.
- Full per-function breakdown captured in `coverage.txt` (from `/tmp/shipcode-c0000-tenancy.cover`).

## Per-task commits

Test-first, one commit per ship-code unit (4 units), plus 2 isolation fixups:

| Commit | Subject |
|--------|---------|
| `7b56a08` | feat: Add the Tenant primitive, tenants table, and tenant_id keying on every scoped table (c0000-tenancy unit 01) |
| `0cda081` | feat: Implement RegisterTenant and thread TenantID through every read/write and listing API (c0000-tenancy unit 02) |
| `29ae469` | feat: Bind registration tokens and authenticated credentials to a tenant and deny cross-tenant access (c0000-tenancy unit 03) |
| `46c0613` | feat: Flip the tenancy e2e scenarios to Green and validate the change (c0000-tenancy unit 04) |
| `280d603` | fix(c0000-tenancy): scope runner list/delete by tenant AND account |
| `512abe3` | fix(c0000-tenancy): enforce tenant isolation at the HTTP layer |

Tasks: 14/14 checked in `tasks.md`.

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
