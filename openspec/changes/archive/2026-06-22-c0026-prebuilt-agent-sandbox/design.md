## Context

The redesign already lands the parts: a pluggable sandbox driver
(`libs/sandbox/engine/{local,docker,cloudflare,custom}`, selected by
`libs/sandbox/runtime/manager.go:NewManagerFor`), agent detection
(`libs/sandbox/agent/detect.go`), a sandbox bundle format
(`libs/sandbox/schema.go`, `libs/sandbox/cmd/mework-sandbox`), the message bus
(`message-bus`), an agent catalog of versioned/immutable artifacts
(`agent-catalog`), and server-side session + conversation + run-event scaffolding
(`libs/server/session`, archived c0009/c0011/c0012). What is missing is the layer
that makes these usable as *products*: there is no named, ready-to-run agent+sandbox
combo, and the runner (`libs/client/runner`) still executes **one-shot** — it builds
a single prompt, calls `Exec(stdin=prompt)` once, captures output, reports a terminal
result. There is no long-lived process, no per-turn input, no streamed output.

This change is the **composition layer**, not new infrastructure. It defines the
prebuilt definition and adds the interactive execution path that drives a long-lived
agent inside a sandbox, reusing the existing contracts.

## Goals / Non-Goals

**Goals:**
- Make agent+sandbox combos **first-class, named, versioned, immutable** definitions
  selectable by reference (`name@version`), binding `{engine, backend, image/config,
  resource limits}`.
- Treat the **sandbox as the agent runner** with a **pluggable engine** (`local`
  first; `docker` one of many; `cloudflare`/`custom` already exist).
- Drive a **long-lived sandbox interactively** from chat: multi-turn over the bus,
  per-turn input over **stdin**, cancel/interrupt, lifecycle + ownership + tenant
  scoping.
- Stream **live logs/observability** (`token|message|done|error` + run telemetry),
  tail-then-live, with queryable session status/list.
- Let container engines consume **pre-baked images** (agent CLI pre-installed).

**Non-Goals:**
- The sandbox driver interface and engine implementations — that is `sandbox-runtime`
  (this change *composes* them; `local` first).
- The transport/bus, session/conversation primitives, run-event kinds, catalog
  artifact forms, and grants — those are `message-bus` / `sessions` / `chat` /
  `run-events` / `agent-catalog` / `auth-and-secrets`. This change reuses, not
  restates, them.
- Authoring a marketplace UI or a rich policy language. A small enumerable set of
  prebuilt definitions ships in-tree; more are added later.
- New isolation guarantees for the `local` engine (it remains trusted-only).

## Decisions

- **A sandbox *has* an engine; the definition picks it.** The conceptual model is
  `daemon → sandbox`, and the engine is a *property* of the sandbox, not a sibling.
  The prebuilt definition records `engine` and the runtime resolves it through the
  existing `NewManagerFor(engine)` dispatch. Adding an engine or a backend requires
  **no schema migration** (consistent with the provider-agnostic invariant).
- **Definitions reuse the sandbox bundle + catalog artifact forms.** A prebuilt
  definition is the existing `SandboxBundleMetadata` (`sandbox.yaml`) extended with
  the engine/backend/image/limits binding, published as an `agent-catalog` artifact so
  it is versioned, immutable, and pullable. No parallel storage system.
- **Definitions are immutable; pointers move.** `name@version` is immutable;
  republishing the same version with different content is rejected; `latest`/channels
  resolve at run time — mirrors `agent-catalog`.
- **Interactive execution = long-lived process + bus turns.** Instead of one
  `Exec(stdin=prompt)`, the daemon starts the sandbox once and keeps the agent process
  alive; each chat turn is fed to the **process stdin**, and stdout/stderr are parsed
  into `token|message|done|error` events published to the session's bus topic. This
  promotes the defined-but-unimplemented `ports.AgentBackend`
  (`libs/shared/ports/interfaces.go`) into the real path (or folds an interactive
  variant onto `SandboxDriver.Exec`).
- **Stdin-not-argv and one-agent-per-sandbox are preserved.** Turn content is
  attacker-controllable; it never reaches argv. The manager already enforces one agent
  per sandbox; a session owns exactly one sandbox.
- **Lifecycle bound shifts from a single 30-min run to the session.** One-shot runs
  keep the 30-min timeout; a long-lived interactive sandbox is bounded by the
  session's lifecycle (explicit close + idle reaping), reusing the server session
  manager's reaper.
- **Observability reuses run-events/chat kinds with tail-then-live.** Late subscribers
  receive buffered backlog then the live stream (reuse the bus's resumable delivery /
  `last_event_id`). Status/list are queryable per tenant.
- **Pre-baked images are pinned by the definition.** For container engines the
  definition's `image` points at an image with the agent CLI already installed; the
  engine pulls/runs it and performs no install step. The `local` engine ignores image.

## Risks / Trade-offs

- **Long-lived process resource leaks** → bound every session by idle reaping +
  explicit close (reuse the session reaper); destroy the sandbox on session close;
  cap concurrent sessions per tenant (reuse quotas where present).
- **Streaming parser fragility (CLI output → events)** → keep a thin, backend-specific
  adapter; on parse failure fall back to a raw `log`/`output` event rather than
  dropping data; one terminal `done`/`error` per turn.
- **Cancel/interrupt semantics** → cancel interrupts the in-flight turn via the
  engine's signal path (`Signals`) without destroying the sandbox; only close/idle
  destroys it. `local` has no signal isolation — document trusted-only.
- **Definition/version sprawl** → reuse `agent-catalog` retention (GC out of scope
  here, noted).
- **Backend skew across engines** (e.g. `windows-claude` vs `local-claude`) → the
  definition pins backend + engine + image together so a reference is reproducible;
  detection (`DefaultBackends`) only seeds defaults.

## Migration Plan

- Additive: existing one-shot dispatch (`libs/client/runner` `Run`/`Engine`) keeps
  working; interactive sessions are a new path opened explicitly. No data migration
  (definitions ride the catalog; no new engine/backend migration).
- Ship a small set of in-tree prebuilt definitions (`local-claude` first, then
  `docker-claude`, `codex-docker`, …) so the feature is usable on day one.

## Open Questions

- Exact wire shape of a "turn" message vs. reusing the `chat` `Conversation.Send`
  payload — prefer reusing `chat` verbatim if it fits.
- Whether `local-claude` interactive mode keeps the CLI in a REPL or re-execs per turn
  with carried history (the `examples/remote-claude` test simulates the latter).
