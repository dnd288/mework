## Why

mework wants to offer **fully remote access to any kind of AI agent**: pick a
ready-to-run agent, open a chat, and drive it from anywhere with live logs and
session management — without first installing the agent CLI, choosing an engine, or
hand-wiring a session. The pieces exist as scaffolding but are not a working whole.
The sandbox driver abstraction (`local`/`docker`/`cloudflare`/`custom`), agent
detection, and a sandbox bundle format are built; sessions, chat (`Conversation`),
and run-events are specified and scaffolded server-side. But the runner still
executes **one-shot** (prompt-in → result-out): there is no notion of a named,
ready-to-run agent+sandbox combo, and nothing drives a **long-lived agent
interactively** with streamed logs.

This change adds a **prebuilt agent-sandbox** layer: named, versioned, ready-to-run
definitions that bind an engine + agent backend + image/config, plus the runtime that
makes the **daemon drive a long-lived sandbox over chat** with live observability.

## What Changes

- A new **prebuilt-agent-sandbox** capability. The conceptual model is `daemon →
  sandbox`, and a **sandbox *has* an engine** — the sandbox is the agent runner; the
  engine (`local` first; `docker` is one of many; `cloudflare`/`custom` already exist)
  is *how/where* it runs. Agent backends are open-ended: `claude`, `codex`,
  `windows-claude`, `v0`, `opencode`, …
- **Prebuilt sandbox definitions** — named, versioned, **immutable** definitions
  binding `{engine, agent backend, image/config, resource limits}` into a ready-to-run
  combo (e.g. `local-claude`, `docker-claude`, `codex-docker`, `windows-claude`, `v0`),
  selectable by reference (`name@version`). Reuses the existing sandbox bundle metadata
  and agent-catalog artifact forms.
- **Run a prebuilt sandbox by reference** — resolve the definition → start a sandbox
  via its engine → run the agent. Prompt/turn content goes over **stdin, never argv**;
  **one agent per sandbox**.
- **Interactive multi-turn chat sessions** — wire the existing `Session` /
  `Conversation` / bus contracts into a **long-lived agent-in-sandbox process**:
  multi-turn, remote-controlled over the bus, cancel/interrupt, ownership + tenant
  scoping. (Today the runner is one-shot — this is the main new behavior.)
- **Live logs & observability** — stream run/session events
  (`token|message|done|error` + run telemetry) back to clients, **tail-then-live** for
  late subscribers, queryable session status/list.
- **Pre-baked container images** — container engines (e.g. `docker`) consume an image
  with the agent CLI pre-installed, pinned by the definition; nothing is installed at
  run time. The `local` engine needs no image.

## Capabilities

### New Capabilities
- `prebuilt-agent-sandbox`: define named/versioned/immutable agent+sandbox combos
  (engine + backend + image/config), run them by reference, drive them as long-lived
  interactive chat sessions with streamed logs and observability, and pin pre-baked
  images for container engines.

### Modified Capabilities
- `daemon-runtime`: the daemon maintains a **long-lived sandbox per session** and
  routes chat turns + **streams events** over the bus, in addition to the existing
  one-shot dispatch. Preserves the stdin-not-argv and one-agent-per-sandbox invariants.

## Impact

- **Sequenced after** `c0006-sandbox-runtime`, `c0009-sessions`, `c0011-chat`,
  `c0012-run-events`, and `c0025-channel-routing` — it composes their contracts rather
  than restating them (`message-bus` topics, `libs/server/session` Session/Conversation,
  run-events kinds, `agent-catalog` artifact forms, `auth-and-secrets` grants).
- **Definition format**: extends the sandbox bundle metadata (`libs/sandbox/schema.go`)
  and bundle tooling (`libs/sandbox/cmd/mework-sandbox`); engine selection reuses
  `libs/sandbox/runtime/manager.go` (`NewManagerFor`).
- **Execution path**: promotes the defined-but-unimplemented `ports.AgentBackend`
  (`libs/shared/ports/interfaces.go`) or folds it onto `SandboxDriver.Exec` for the
  long-lived path; bridges `libs/server/session` into `libs/client/runner` (replaces the
  one-shot `Exec(stdin=prompt)` with a long-lived process fed per turn).
- **Backends**: extends `libs/sandbox/agent/detect.go` `DefaultBackends`
  (`windows-claude`, `v0`). Domain types `core.RunSpec` / `core.SandboxCaps`
  (`libs/shared/core/types.go`) gain definition-reference and session fields as needed.
- **Engine ordering**: `local` engine first; `docker` is one additional engine;
  `cloudflare`/`custom` already exist. No schema migration is required to add an engine
  or a backend.
