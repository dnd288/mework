## Why

The target DX mirrors a GitHub Actions self-hosted runner: an operator installs a
runner once, then drives **all** subsequent work from the hub without touching the
machine — including shipping **new agents** to it on demand and constraining what
those agents may do. Today there is no notion of a distributable agent: `profiles`
is static per-account config (`server/catalog`), there is no versioning,
no pull endpoint, and no permission model. Operators cannot "pull a new agent" or
scope "permitted operations".

This change adds the **hub side** of that DX: a catalog of versioned, pullable
agents, a dispatch mechanism, and a permission/policy model that scopes what a
dispatched agent may do.

## What Changes

- A new **agent-catalog** capability: agents are **versioned, pullable artifacts**;
  the hub can publish, list, resolve a version, and serve (pull) an agent, and
  **dispatch** an agent to a session/runner.
- Agent artifacts are **type-agnostic**: a definition/manifest (prompt + workflow +
  declared needs) or a packaged/container image reference; the consumer
  (sandbox driver) decides how to materialize it.
- A **permission/policy model**: each dispatch carries a scoped grant of permitted
  operations; the hub authorizes, and the grant travels with the dispatched work so
  the runner/sandbox can enforce it.

## Capabilities

### New Capabilities
- `agent-catalog`: publish/list/resolve/pull versioned agent artifacts, dispatch an
  agent to a target, and attach a scoped permission grant ("permitted operations").

### Modified Capabilities
- `auth-and-secrets`: authentication is extended with **scoped permission grants**
  attached to runner/agent/session identities.

## Impact

- **Sequenced after `c0002-repo-restructure`** (and `c0002-message-bus`): catalog/permission
  code lands in `server/{catalog,permission}`; grant/agent DTOs in
  `shared/transport`; sealing via `server/platform/secret`/`token`.
- New server routes under `/api/v1/agents` (publish, list, resolve, pull) and a
  dispatch path that publishes to a runner/session topic (see `message-bus`).
- New persistence: `agents` (catalog entries + versions) and a permissions/grants
  representation; reuses `server/platform/secret` for any sealed artifact creds.
- `profiles` is subsumed/extended by catalog agent definitions (migration path).
- Consumed by `agent-runner` (pull + enforce) and `sandbox-runtime` (materialize).
