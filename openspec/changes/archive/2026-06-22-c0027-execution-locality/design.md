## Context

mework's hybrid model (CLAUDE.md) keeps AI work local: the central server routes and
stores, the runner executes. c0026 built the prebuilt-agent-sandbox execution path on
the runner but did not write the boundary into a spec, and the remote-claude README
diagram blurs it. This change records the boundary normatively; it is doc/spec only.

## Goals / Non-Goals

**Goals:**
- State, normatively and testably, that the daemon + sandbox run on the runner and the
  server is a gateway + registry only (never spawns a sandbox / executes an agent).
- Correct the remote-claude README diagram to match.

**Non-Goals:**
- Any production behavior change — the code already enforces this.
- A new capability — the requirement belongs in the existing prebuilt-agent-sandbox
  capability that owns the execution model.

## Decisions

- **Put the requirement in `prebuilt-agent-sandbox`**, not a new capability or
  daemon-runtime: c0026 already houses the execution model there, keeping the boundary
  next to the behavior it constrains (DRY/KISS).
- **Make it enforceable** with an import-guard rule (`libs/server/**` must not import
  `mework/libs/sandbox/engine/*` or `…/runtime`), turning the "server never spawns a
  sandbox" scenario into a compile-time/test-time check rather than prose only.
- **Definitions are published to the server registry but executed on the runner** — the
  catalog stores; the runner resolves + materializes + runs.

## Risks / Trade-offs

- [Import-guard too strict] → scope the rule to engine/runtime packages only; the server
  may still import `core`/`ports`/`schema` types it legitimately needs.
- [Diagram drift] → keep the README change minimal (only the box layout), so it stays
  easy to review and maintain.
