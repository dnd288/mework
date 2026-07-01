## Why

The c0026 post-ship review flagged a **required** security finding: `Session.List` was
the only session operation not caller-authorized. It took a raw `tenant` argument and
returned that tenant's sessions with no check, so a caller could pass any tenant ID and
read another tenant's session list — while the doc comment and test asserted a tenant
scoping the code did not enforce. The code was fixed on `main`
(`fix(prebuilt-agent-sandbox): authorize Session.List`), but the spec still omits `list`
from the authorized operations. This change brings the spec in line so the guarantee is
captured and shippable through OpenSpec.

## What Changes

- Modify the **prebuilt-agent-sandbox** "Remote-control authorization" requirement to:
  - include **`list`** in the set of authorized session operations, and
  - require that for listing, the tenant scope is **derived from the authenticated
    caller**, never from a caller-supplied argument.
- Add a scenario asserting list returns only the caller's own tenant's sessions
  regardless of any supplied tenant argument.

The production code already implements this (`List(ctx, caller Caller)` verifies the
grant and uses `caller.Tenant`); this change is the spec catch-up plus its tests.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: tightens **Remote-control authorization** to cover `list`
  and mandate caller-derived tenant scope.

## Impact

- **Sequenced after** `c0026-prebuilt-agent-sandbox`.
- Spec delta only; code already on `main`:
  `libs/client/runner/interactive_session.go` (`List` takes a `Caller`, verifies the
  grant, derives tenant from the caller), with cross-tenant and no-grant coverage in
  `libs/client/runner/session_events_test.go` and the e2e helper updated.
- No schema or API change. The server HTTP handler already enforced
  `auth.GetAccountID`; this closes the in-process/client path.
