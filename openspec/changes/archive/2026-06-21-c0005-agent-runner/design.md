## Context

The daemon today (`client/runner/run.go`) is a stateless poll worker keyed to a
pre-registered `runtime` row; it has no enrollment ceremony and no concept of
pulling/running a dispatched agent. The `message-bus` change gives it an SSE
subscription; `agent-catalog` gives it something to pull and a grant to enforce.
This change turns the daemon into a GitHub-Actions-style runner.

## Goals / Non-Goals

**Goals:**
- One-time enrollment producing a durable, persisted runner identity.
- Fully unattended operation after install (no manual host operation).
- SSE subscription + presence; push-driven dispatch.
- A pull → run → report loop with dispatch acknowledgement.
- Client-side enforcement of the dispatched permission grant.

**Non-Goals:**
- The SSE transport internals (`message-bus`).
- The catalog, dispatch, and grant issuance (`agent-catalog`).
- Sandbox driver internals (`sandbox-runtime`) — the runner *invokes* a sandbox.

## Decisions

- **Enrollment like `actions/runner config`.** A registration token (short-lived,
  issued by the hub) is exchanged once for a durable runner credential stored at
  `~/.mework/` with `0600`. Registration tokens are not reusable as the long-lived
  identity.
- **Unattended by default.** After enrollment the runner needs no flags to receive
  work; `daemon start` opens the SSE subscription and runs the loop.
- **Pull is lazy.** The runner pulls the agent artifact only on dispatch, keeping
  idle runners cheap and supporting large/image artifacts.
- **Defense in depth on grants.** The hub authorizes, but the runner **also**
  enforces the grant locally and the sandbox contains — three layers so a
  compromised hub message or buggy agent cannot exceed scope.
- **Identity separation.** Runner identity (host enrollment) is distinct from a
  session (a live agent association) and from a runtime row; presence is tracked on
  the SSE channel.

## Risks / Trade-offs

- **Enrollment token handling.** Registration tokens must be short-lived and
  single-use to limit blast radius if leaked.
- **Reconnect storms.** Many runners reconnecting after a hub blip; mitigate with
  jittered backoff and `Last-Event-ID` resume.
- **Local grant enforcement coverage.** The runner can only enforce operations it
  mediates; operations performed entirely inside a sandboxed agent rely on the
  sandbox boundary (`sandbox-runtime`) — hence defense in depth.
- **Migration.** Existing registered runtimes need an enrollment path or a
  compatibility shim so current installs are not orphaned.
