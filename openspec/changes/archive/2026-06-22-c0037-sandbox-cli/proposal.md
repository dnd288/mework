## Why

With the hub runnable in-process (c0035) and sessions bindable to a local workspace (c0036),
the last piece of the streamlined three-command UX is the ergonomic front door:
**`mework sandbox start -w .`** — stand in a workspace folder and turn it into a running,
server-addressable worker you can message by id. Today there is no `sandbox` command at all;
a user would have to hand-craft a `session create` body with the workspace path and look up
the local runner id.

This change adds a `sandbox` command group that wraps the shipped session API for the
workspace-as-worker flow, completing: `mework server start` → `mework daemon start` →
`mework sandbox start -w .` → message the worker by id.

## What Changes

- **`mework sandbox start -w <dir>`** (default `.`): resolve `<dir>` to an absolute path;
  require `<dir>/mework.yml` and read its `name`/`version`
  (`catalog.LoadWorkspaceConfig`); resolve the **local daemon's** runner id via
  `config.LoadIdentity()` (error with guidance if not enrolled); `POST /api/v1/sessions`
  (PAT auth, like `cmd_session.go`) with `{agent_name, version, runner, workspace}` so the
  server dispatches a workspace-bound open-session to the local daemon (c0036), which opens
  the sandbox bound to the dir. Print the session id (`--json` for scripting). `--attach`
  streams the session immediately.
- **`mework sandbox list`** / **`mework sandbox stop <id>`** — thin aliases over
  `GET /api/v1/sessions` and `DELETE /api/v1/sessions/{id}`.
- **`mework sandbox send <id> <message>`** — alias of the shipped `session send` so "send a
  message to the running worker by id" is discoverable from the `sandbox` group. (The worker
  alternatively subscribes for events via the existing channel/input subscription.)
- Registered under the Runner group alongside `daemon`, `session`, and `server`.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `cli`: adds a `sandbox` command group — `sandbox start -w <dir>` turns a local workspace
  into a running, server-addressable worker (via the local daemon), and `sandbox
  list/stop/send` manage and message it by id.

## Impact

- **Client code:** `libs/client/cli/cmd_sandbox.go` (new `sandbox` group + commands);
  `help.go` registration. Reuses the HTTP/SSE client helpers and PAT resolution from
  `cmd_session.go`.
- **Reuses** `catalog.LoadWorkspaceConfig` (`libs/client/catalog/file_resolver.go`),
  `config.LoadIdentity` (`libs/shared/config/identity.go`), the session HTTP routes
  (c0031/c0032) and the workspace-bound dispatch (c0036).
- **Depends on** c0036 (server forwards `workspace`) and the shipped session API; benefits
  from c0035 (`server start`) for the full local loop. No server change.
- No schema migration. Same-machine/user assumption for the workspace path (documented in
  c0036).
