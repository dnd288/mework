## Context

The current event flow in `mework-server` is:

```
Webhook → ParseTrigger → ResolveProfile → Publish("runner.<profile>.dispatch") → SSE → Runner claims → Sandbox
```

This couples event routing to **profile names** (e.g. `dev`, `senior-dev`). Two tickets using the same profile collide on the same topic. There is no per-resource isolation, no provider-agnostic routing, and no way to correlate multiple events for the same external resource (Mello ticket, GitHub issue, Jira ticket) to the same sandbox session.

**What already exists:**
- **Agent catalog**: `POST /api/v1/agents/{name}/versions` pushes sandbox definitions; `GET /api/v1/agents/{name}/versions/{version}/pull` pulls them; `POST /api/v1/agents/{name}/dispatch` sends them to a named runner. This is the push/pull/distribute fabric — any machine can publish a sandbox, any other machine can pull and run it.
- **Runner enrollment**: any machine authenticates and enrolls with a registration token. Runners stay connected via SSE.
- **Session manager**: creates/attaches/closes sessions with bus-backed control channels.
- **Message bus**: topic-based pub/sub with SSE, resumable delivery, wildcard matching.

**What's missing:**
- A **channel routing layer** that routes events by `(provider, resource_id)` rather than profile name, enabling any online worker to pick up work for any resource without being pre-assigned.
- **Auto-provisioning**: when an event arrives for a resource that has no active worker session, automatically select a worker, pull the agent, spawn the sandbox.
- **Spec-aware worker selection**: the channel router selects any worker whose declared specs match the task's requirements.

## Goals / Non-Goals

**Goals:**
- Route incoming events from any provider to a worker session scoped by `(provider_code, external_resource_id)`
- Any online worker that declares matching specs can be selected for any channel — workers are interchangeable within their spec class
- Auto-provision a sandbox on the selected worker when no session exists for a channel
- Deliver all events for one resource through one channel to one sandbox (serialized per resource)
- Enable parallelism across resources (different tickets → different channels → concurrent sandboxes on different workers)
- Provider-agnostic channel key computation; the router never knows Mello vs GitHub vs Jira
- Reuse the existing agent catalog for sandbox push/pull/dispatch — the catalog IS the sandbox registry
- Support a portable **zip bundle format** for sandboxes: a self-contained folder with metadata, definition, tools, hooks, and assets, all packed into a single zip file published to the catalog

**Non-Goals:**
- Immediate replacement of the existing `runner.<profile>.dispatch` topic (deprecated but kept for migration)
- Cross-datacenter or cross-region channel routing (single-server model)
- Persistent event log beyond channel lifetime (channels retain events only while a session is bound)

## Decisions

### Decision 1: Workers are interchangeable within spec class

Any machine that authenticates, enrolls, declares its specs, and stays connected via SSE is a worker. When a channel needs a sandbox, the auto-provisioner selects **any** online worker whose specs match the task — not a specific pre-assigned runner. This is the key difference from the current model (where a specific runner claims a job).

```
Worker A (MacBook, specs: [claude-code])
Worker B (Linux box, specs: [claude-code, codex])
Worker C (Windows, specs: [opencode])

Channel fires for spec claude-code → picks A or B (fewest active channels)
Channel fires for spec opencode → picks C (only match)
```

This makes workers fungible within their capability class — like worker pools in CI/CD systems.

#### Scenario: Any matching worker can serve any channel

- **WHEN** two channels fire concurrently, both requiring spec `claude-code`, and workers A and B both support it
- **THEN** A gets one channel, B gets the other — load balanced by fewest active bindings

#### Scenario: No matching worker → event buffered

- **WHEN** a channel fires for spec `codex` but no online worker supports it
- **THEN** the event is buffered in the channel topic and the router retries with backoff

### Decision 2: Channel key format `(provider_code, resource_id)`

The channel key is a two-part tuple: `provider_code` (e.g. `mello`, `github`, `jira`) and `external_resource_id` (the provider's native ID for the ticket/issue/PR). In-memory representation is a colon-joined string: `mello:ticket-abc123`. Bus topics use dot-segments: `channel.mello.ticket-abc123.event_type`.

### Decision 3: Auto-provisioning flow

When `ChannelRouter.Route()` finds no active session for a key:

1. **Resolve spec** from the event's profile (`backend_hint` → maps to agent catalog name)
2. **Select worker** by querying `runtimes WHERE status = 'online' AND specs @> ARRAY[spec]`, ordered by fewest active channel bindings
3. **Create session** via `session.Manager.Create()` (which bus-subscribes the session)
4. **Bind channel → session** in `channel_sessions` table
5. **Dispatch agent to worker**: call the agent catalog's `Dispatch` endpoint with the selected worker and the resolved agent from the profile
6. **Worker pulls agent** from the catalog via `PullVersion` (with scoped grant)
7. **Worker spawns sandbox** and subscribes to `channel.<provider>.<resource_id>.*`
8. **Replay buffered events** from the channel topic
9. **Deliver current event** to the sandbox

### Decision 4: Channel sessions backed by Postgres, cached in memory

`channel_sessions` table:
```sql
CREATE TABLE channel_sessions (
    channel_key   TEXT PRIMARY KEY,
    session_id    UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    provider_code TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    runner_id     UUID NOT NULL REFERENCES runtimes(id),
    spec          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'draining', 'closed')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at     TIMESTAMPTZ
);
```

In-memory `sync.Map` caches active channel→session for zero-DB hot path. Populated from DB on startup.

### Decision 5: Sandbox bundle format — portable zip with definition + metadata + tools

To make sandboxes fully self-contained and portable between machines, the catalog SHALL support a **"bundle"** form alongside the existing `"definition"` and `"image"` forms. A bundle is a zip file whose internal layout mirrors a standardized folder structure:

```
sandbox-bundle.zip
├── sandbox.yaml              # REQUIRED: metadata — name, version, spec, backend, author
├── definition.md             # REQUIRED: agent prompt / system message
├── tools/                    # OPTIONAL: MCP tools or plugins
│   └── <tool-name>/
│       ├── manifest.yaml
│       └── handler.*
├── hooks/                    # OPTIONAL: lifecycle scripts
│   ├── setup.sh              #   runs before sandbox starts
│   ├── teardown.sh           #   runs after sandbox finishes
│   ├── pre-run.sh            #   runs before each event
│   └── post-run.sh           #   runs after each event
├── assets/                   # OPTIONAL: reference data, images, templates
└── config/                   # OPTIONAL: default overrides
    ├── env.yaml              #   environment variables
    └── limits.yaml           #   resource limits (CPU, memory, timeout)
```

The worker:
1. Pulls the zip from the catalog via `PullVersion`
2. Extracts it to an isolated workdir
3. Reads `sandbox.yaml` for metadata (spec, backend)
4. Reads `definition.md` for the agent system prompt
5. Mounts `tools/` into the sandbox's MCP tool registry
6. Runs lifecycle hooks from `hooks/` at the appropriate stages
7. Passes config from `config/` as runtime defaults (overridable by dispatch params)

The zip is stored as `payload BYTEA` in the `agent_versions` table, exactly like the existing `"definition"` form stores its text — no schema change needed, only a new form identifier.

### Decision 6: Agent catalog as the sandbox registry

The existing agent catalog provides the push/pull/dispatch fabric. The channel router uses it as follows:

- **Push**: A developer creates a sandbox bundle locally (a folder with the standard layout), zips it, and calls `POST /api/v1/agents/{name}/versions` with `form: "bundle"` and the zip as payload.
- **Pull**: When a worker is dispatched for a channel, it calls `GET /api/v1/agents/{name}/versions/{version}/pull` with its scoped grant. The server returns the zip bytes plus the form indicator. The worker extracts and materializes the sandbox.
- **Dispatch**: The auto-provisioner calls `POST /api/v1/agents/{name}/dispatch` with the selected worker as the target and a grant scoped to pull the agent and process the channel.

No new storage or registry is needed — the agent catalog handles all three forms (`definition`, `image`, `bundle`) through the same versioned artifact model.

### Decision 7: Write-back via channel session context

When the sandbox completes processing, the write-back flow uses the channel session context (account ID + provider code) to look up the provider connection and decrypt the token server-side. The worker never holds the provider token.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| **Double-provisioning on restart**: bindings lost before cache populated | Populate cache from DB on startup; advisory lock per channel key during provisioning |
| **No eligible worker for spec** | Buffer in channel topic; retry 3× with 5s backoff; if still none, return 200 to provider for provider-side retry |
| **Orphaned channel sessions** | Sweeper (30s interval) closes channels whose session/runner is gone |
| **Agent catalog is single-server** | Already Postgres-backed; scales with the server |

## Migration Plan

1. Add `specs TEXT[]` column to `runtimes` (nullable — NULL means "all specs")
2. Create `channel_sessions` table
3. Deploy `ChannelRouter`, `ChannelRegistry`, `AutoProvisioner` behind feature flag
4. Update worker enrollment to accept `specs` field
5. Extend bus topic matching for `channel.*.*.*` (already supported via `**`)
6. Wire `ChannelRouter.Route()` into webhook handler alongside existing path
7. Update daemon to enroll with specs and subscribe to channel topics
8. Remove feature flag, deprecate `runner.<profile>.dispatch`

## Open Questions

1. Should channel routing work for non-ticket resources (board-level webhooks)?
2. Max concurrent channels per worker? Per-runner cap via `runtimes.max_channels`?
3. Do we need a `ChannelPing` mechanism for workers to declare they're still alive on a channel, separate from the runner heartbeat?
