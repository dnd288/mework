## 1. Spec

- [ ] 1.1 Modify the prebuilt-agent-sandbox "Remote-control authorization" requirement to include `list` and the caller-derived-tenant rule (done in this change's delta)

## 2. Code (already on main — verify)

- [ ] 2.1 `Session.List` takes a `Caller`, verifies the grant, and derives the tenant from `caller.Tenant` (never a supplied argument) — `libs/client/runner/interactive_session.go`
- [ ] 2.2 Callers updated to the new signature (CLI / e2e helper)

## 3. Tests

- [ ] 3.1 List returns the caller's own tenant sessions; a caller in another tenant sees none — `libs/client/runner/session_events_test.go`
- [ ] 3.2 List without a grant is denied

## 4. Validation

- [ ] 4.1 `openspec validate c0028-session-list-tenant-auth --strict` passes
- [ ] 4.2 `make vet` and `make test` green
