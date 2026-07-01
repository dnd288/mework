# Runtime & Sandbox

> Audience: developers working on the client side (the daemon/runner) and execution.
> Covers the runner lifecycle (today's poll loop and the target SSE loop), AI CLI
> detection, and the pluggable sandbox model. Status badges: **`[Implemented]`** today;
> **`[Planned — cNNNN]`** specified under `openspec/changes/`.

## The runner, today and target

The client binary (`cmd/mework`) hosts a long-lived **daemon** (becoming a **runner**)
that executes AI work locally. The *what* is unchanged — read a ticket, run an AI CLI,
report a result. The *how it receives work* is what the redesign changes.

### Today: the poll loop `[Implemented]`

`internal/daemon/run.go`. Requires `server_url` and `rt_token` in config. Detects an AI
backend (retries each tick if none found), then on each tick:

1. **Claim** — `POST /api/v1/jobs/claim` every `poll_interval_seconds` (default 5s)
   with the `rt_token`. `204` → nothing to do.
2. **Ack running** — `POST /jobs/{id}/ack` status `running`.
3. **Heartbeat** — a background goroutine `POST /jobs/{id}/heartbeat` every 30s to
   extend the lease.
4. **Run** — build the prompt, execute the AI CLI (30-minute cap).
5. **Ack terminal** — `POST /jobs/{id}/ack` status `done`/`failed` with the summary.

The **server** then writes the result back over the provider's REST API — the daemon
never touches provider credentials.

### Target: enroll → subscribe → pull → run → report `[Planned — c0004]`

The daemon becomes an **enrolled SSE runner**, modeled on `actions/runner`. It must
**not** poll a claim endpoint on a fixed interval; when idle it holds an open SSE
subscription and reports presence.

1. **Enroll once** — `mework runner enroll --url <hub> --token <registration-token>`
   exchanges a short-lived, single-use registration token for a **durable runner
   identity**, persisted at `~/.mework/` with `0600` perms. Invalid/expired tokens are
   rejected and **no identity is persisted on failure**.
2. **Subscribe** — on `daemon start`, open the SSE subscription to the runner's topics;
   maintain presence/heartbeat over the channel; reconnect with **jittered backoff +
   `Last-Event-ID` resume**.
3. **Pull** — on a dispatch (received by push, no polling), resolve and **pull** the
   referenced agent version from the catalog — lazily, only on dispatch.
4. **Run** — execute it via the sandbox runtime (below) under the dispatched grant.
5. **Report** — POST the terminal result (`done`/`failed` + summary) and **acknowledge**
   the dispatch so it isn't redelivered.

**Grant enforcement (defense in depth):** the hub authorizes → the runner parses and
verifies the integrity-checked **grant** and refuses operations outside scope (it can
never widen its own scope) → the sandbox contains anything the runner doesn't mediate.
See [auth-and-secrets.md](auth-and-secrets.md).

**Identity separation:** the runner identity (host enrollment) ≠ a session (a live
agent association) ≠ a runtime row. Presence is tracked on the SSE channel. A
migration/compat path is provided for existing registered runtimes.

## Daemon lifecycle `[Implemented]`

| Command | Behavior |
|---------|----------|
| `mework daemon start` | Re-execs detached in the background (`--foreground` runs in-process). No-op if already running |
| `mework daemon stop` | Graceful shutdown via the local health port; falls back to SIGTERM |
| `mework daemon status` | Reports running/stopped, pid, and health port |
| `mework daemon restart` | Stop (if running) then start |
| `mework daemon logs [-f]` | Print (and optionally follow) the daemon log |

State files live under the profile directory (default `~/.mework/`, or
`~/.mework/profiles/<name>/` with `--profile`):

- `daemon.pid` — running process id (liveness via signal 0, so a stale file after a
  crash isn't mistaken for a live daemon).
- `daemon.log` — daemon output.
- `work/<job-id>/` — isolated working directory per agent run (`0700`).

The health/shutdown port is derived deterministically from the profile name
(`19514 + fnv32a(profile)%1000`), so each profile gets its own port without config.
Idempotency and loop prevention are entirely **server-side** (unique constraints +
advisory locks), so the daemon keeps no local `state.json`.

## AI CLI detection & execution `[Implemented]`

**Detection** (`internal/agentrun/detect.go`): `DefaultBackends = [claude, codex,
opencode]` (overridable via config `daemon.backends`). `Detect` returns the first on
`PATH` via `exec.LookPath`.

**Execution** (`internal/agentrun/runner.go`): runs the backend binary with the prompt
fed via **STDIN only** — never argv/shell, because ticket content is
attacker-controllable. This is the command-injection control. `cmd.Dir` is an isolated
per-job work dir (`<profileDir>/work/<jobID>`, `0700`); stdout+stderr are merged;
default timeout 30 minutes.

**Prompt building** (`internal/daemon/handler.go`): concatenates the
`profile_body_snapshot` (if any) + `Task Title:` + `Description:` + `Workflow:` (if
set) + `Instructions:`. `formatResult` wraps agent output in a fenced block for
write-back (success truncated at 8000 chars, failure at 4000).

## Pluggable sandboxes `[Planned — c0005]`

Today, execution is a bare `exec.CommandContext(runCtx, b.Path)` in a `0700` per-job
dir, running as the daemon's own OS user with **full host access** — the single seam at
`internal/agentrun/runner.go`. That is fine for code you trust but unsafe for arbitrary
hub-dispatched agents. The sandbox redesign replaces that seam with a pluggable driver.

### Driver interface

In the new `internal/client/sandbox` package (was `internal/agentrun`):

- **Lifecycle:** `create → start → exec → stop → destroy`.
- **`Driver.Run(ctx, spec) → result`** replaces the bare exec seam.
- **`RunSpec`:** agent ref/command, workdir, env scope, resource limits, timeout.
- **`Result`:** captured stdout/stderr + exit status.
- The prompt is fed over **stdin, never argv** (preserves the existing security
  invariant).

### Drivers

| Driver | Isolation | Use |
|--------|-----------|-----|
| **`local`** (`sandbox/local`) | None — host subprocess in an isolated workdir, stdin prompt, 30-min default timeout | Default for **trusted** use; documented as providing no host isolation (only a working directory). This is today's behavior, formalized behind the interface |
| **`docker`** (`sandbox/docker`) | **Container per agent** — mounts only the provisioned workdir, scopes network/env, applies CPU/memory limits + wall-clock timeout, streams the prompt to container stdin | For running operator-dispatched/untrusted agents. The Docker client dependency is **driver-gated**, so `local`-only builds add zero new deps |

### Lifecycle & selection

- The driver is selected **per-dispatch or by config** (default + override rules).
- **One agent per sandbox** — created on run, destroyed on terminal state (cleanup
  guaranteed even on failure); no shared sandbox state between runs.
- A common resource-limit subset (CPU, memory, timeout) with driver-specific
  extensions allowed. A resource or timeout breach terminates the run and reports
  failure (the dispatch is reported `failed`).

See [architecture.md](architecture.md) for how the runner and sandbox sit in the agent
hub, and the [openspec change](../openspec/changes/) `c0005-sandbox-runtime` for the
full spec.
