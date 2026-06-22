## Why

c0026 introduced the prebuilt agent-sandbox execution model but left the **deployment
boundary implicit**. The code already enforces it — `libs/server` imports no sandbox
engine/runtime and never spawns a sandbox; the daemon spawns and executes the sandbox
on the runner (`libs/client/runner` + `libs/sandbox`) — but no spec states it, and the
`examples/remote-claude` README diagram draws the daemon **and** the sandbox *inside*
the `mework-server` box, which reads as if the server runs the agent. Make the boundary
explicit and normative so it cannot be misread.

## What Changes

- Add a requirement to **prebuilt-agent-sandbox**: the daemon and the sandbox run on
  the **runner**; `mework-server` is a **gateway + registry** only (webhook ingress,
  the agent/definition catalog/registry, session metadata, and the message-bus topics)
  and never spawns a sandbox or executes an agent.
- Fix the `examples/remote-claude/README.md` architecture diagram so the daemon +
  sandbox sit in their own runner tier, with the server box holding only
  sessions/registry/bus.

No production behavior changes — this captures and protects the existing boundary
(optionally enforced by an import-guard rule that `libs/server` must not import the
sandbox engine/runtime).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: adds a normative **Runner-side execution; server is gateway
  and registry** requirement.

## Impact

- **Sequenced after** `c0026-prebuilt-agent-sandbox`.
- Docs/spec + example only: `openspec/specs/prebuilt-agent-sandbox/spec.md` (via delta),
  `examples/remote-claude/README.md`.
- No schema or runtime change. Verified against code: `libs/server/session` imports only
  `bus`, `auth`, `core`, `ports`; sandbox spawning lives in `libs/client/runner` +
  `libs/sandbox/runtime`.
- Optional enforcement: add a `libs/tools/import-guard` rule forbidding
  `libs/server/**` from importing `mework/libs/sandbox/engine/*` or
  `mework/libs/sandbox/runtime`.
