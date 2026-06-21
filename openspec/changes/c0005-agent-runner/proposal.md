## Why

The product goal is the **GitHub Actions self-hosted runner DX**: install once,
then everything is remote-driven — no one SSHes into the device again. Today the
daemon is a **stateless poll worker** (`client/runner/run.go`) with no durable
identity, no enrollment step, and no concept of pulling and running an
operator-dispatched agent. It only polls for jobs tied to a pre-registered runtime.

This change defines the **client side** of the DX: a one-time **enrollment**, then
an unattended runner that subscribes over SSE, **pulls** dispatched agents from the
catalog, runs them, enforces the dispatched permission grant, and reports results —
all without manual operation on the host.

## What Changes

- A new **agent-runner** capability: one-time enrollment producing a durable runner
  identity; an SSE subscription to the runner's topics; a **pull → run → report**
  loop; presence/heartbeat over the channel; grant enforcement.
- The daemon stops polling and becomes an enrolled SSE subscriber that owns local
  sessions and (via `sandbox-runtime`) local sandboxes.
- New CLI surface for install-once enrollment and read-only inspection of
  agents/sessions.

## Capabilities

### New Capabilities
- `agent-runner`: install-once enrollment, unattended SSE subscription, pull/run/report
  loop, presence, and grant enforcement on the client device.

### Modified Capabilities
- `daemon-runtime`: the daemon changes from a stateless long-poll worker to an
  **enrolled SSE runner** driving a pull/run/report loop.
- `cli`: add `runner enroll` and agent/session inspection commands; remove
  poll-oriented framing.

## Impact

- **Sequenced after `c0002-repo-restructure`** (and `c0002-message-bus`, `c0004-agent-catalog`):
  runner code lands in `client/{runner,subscribe,cli}`; config in
  `shared/config`; dispatch/grant DTOs in `shared/transport`.
- Affected code: `client/runner` (was `client/runner`),
  `client/subscribe` (was `client/subscribe`),
  `client/cli` (enroll/agent/session commands), `shared/config`.
- Depends on `message-bus` (SSE) and `agent-catalog` (pull + grant); produces work
  for `sandbox-runtime` (execution).
- Removes the poll loop and the `claim`-based client path.
