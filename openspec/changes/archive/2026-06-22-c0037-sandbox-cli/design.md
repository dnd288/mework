## Context

c0036 lets `POST /sessions` carry a workspace path that the daemon binds. c0037 is the CLI
that produces that request from "I'm standing in a workspace folder," plus thin manage/message
verbs. It reuses the session HTTP client already built for `mework session`.

## Goals / Non-Goals

**Goals:**
- `sandbox start -w .` → a running, server-addressable worker bound to the local dir, id
  printed.
- `sandbox list/stop/send` for the basic worker loop.

**Non-Goals:**
- Any server/daemon change (done in c0035/c0036 and the shipped session API).
- A standalone (no-daemon) worker mode — `sandbox start` dispatches to the local daemon.
- Re-implementing streaming — `--attach` and `sandbox send` reuse the `session` client.

## Decisions

- **Target runner = the local enrolled identity.** `config.LoadIdentity()` yields the
  daemon's runner id on this machine; `sandbox start` uses it as the dispatch target so the
  *local* daemon opens the sandbox. If not enrolled (no identity), fail fast with guidance
  ("run `mework runner enroll` / `mework daemon start` first").
- **Validate the workspace locally first.** Require `<dir>/mework.yml`
  (`catalog.LoadWorkspaceConfig`) and resolve `<dir>` to an absolute path before calling the
  server, so errors are immediate and the daemon receives a real path.
- **Thin wrappers over the session API.** `start` = `POST /sessions` with `workspace`;
  `list` = `GET /sessions`; `stop` = `DELETE /sessions/{id}`; `send` = the shipped
  `session send`. Share the base-URL + PAT resolution helper from `cmd_session.go` (extract
  if needed). `--attach` reuses the session stream reader.
- **`sandbox` vs `session`.** `sandbox` is the workspace-oriented façade (start a worker from
  a folder); `session` remains the lower-level session API. `sandbox send`/`stop`/`list` are
  aliases for discoverability; the docs point power users at `session`.

## Risks / Trade-offs

- [Daemon not running] → the session is created on the server but no sandbox opens until a
  daemon for that runner connects. `sandbox start` should note this; optionally warn if the
  identity exists but the daemon health port is down.
- [Abs path on a different host] → out of scope; same-machine assumption documented in c0036.
- [Two ways to message] → `sandbox send` and `session send` both exist; keep `sandbox send` a
  literal alias to avoid divergence.

## Migration Plan

Additive CLI surface. New `sandbox` group; no change to existing commands.
