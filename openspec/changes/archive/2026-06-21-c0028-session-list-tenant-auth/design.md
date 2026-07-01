## Context

A c0026 post-ship review found `Session.List` was the only session op without
caller authorization: it accepted a raw tenant argument and returned that tenant's
sessions, so any caller could read another tenant's list. The code was fixed on `main`
(`List(ctx, caller Caller)` verifies the grant and uses `caller.Tenant`); this change
records the guarantee in the spec so it ships through OpenSpec.

## Goals / Non-Goals

**Goals:**
- Make `list` an authorized, tenant-isolated session operation in the spec.
- Require the list tenant scope to come from the authenticated caller, not an argument.

**Non-Goals:**
- New code — the fix already landed on `main`; this is the spec catch-up + its tests.
- A distinct grant operation for list vs other ops — `list` is authorized under the
  existing session grant (OpSpawn), consistent with send/cancel/close/status.

## Decisions

- **MODIFY the existing "Remote-control authorization" requirement** rather than add a
  new one — listing is part of the same authorization concern (DRY).
- **Tenant derived from caller, never supplied** — the only safe contract; a supplied
  tenant is ignored/rejected.

## Risks / Trade-offs

- [Spec/code drift until shipped] → the baseline omits `list` until this change syncs;
  the code already enforces it, so the drift is conservative (code stricter than spec).
