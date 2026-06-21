## Why

The current event routing model dispatches work by **profile name** (`runner.<profile>.dispatch`), which means two different tickets using the same profile land on the same topic with no resource-level isolation. There is no mechanism to route an event from any provider (Mello, GitHub Issues, Jira, etc.) to a dedicated sandbox session scoped to that specific external resource. This prevents true per-resource parallelism, makes it impossible to sequence events per resource, and couples routing to profile names rather than the natural unit of work: the external resource (ticket, issue, etc.).

The agent catalog already supports **push/pull/dispatch** — sandbox definitions can be published to the server, pulled by workers, and dispatched to specific runners. What's missing is a **channel routing layer** that routes events from any provider to the right worker session based on the resource being worked on, and auto-provisions sandboxes when no worker is yet handling that resource.

## What Changes

Introduce a **channel routing** layer that sits between external event sources and the existing agent catalog / message bus infrastructure:

- **Sandbox bundle format** — a portable, self-contained sandbox format: a folder with `sandbox.yaml` (metadata), `definition.md` (agent prompt), `tools/`, `hooks/`, `assets/`, and `config/` — all packed into a zip file. Published to and pulled from the agent catalog as a new `"bundle"` form alongside the existing `"definition"` and `"image"` forms.

- **`ChannelRouter`** — a new component that computes a channel key `(provider_code, external_resource_id)` from every incoming event, looks up the active worker session for that channel, and either delivers the event or triggers auto-provisioning of a sandbox worker.

- **`ChannelRegistry`** — maps channel keys to active worker sessions. Persisted in Postgres for durability with an in-memory cache.

- **`AutoProvisioner`** — when no worker session exists for a channel, selects an eligible online worker (by spec/agent capability match), delivers the agent definition from the catalog, spawns the sandbox on the worker, creates a session, binds the channel, and delivers buffered events.

- **Runner → Worker** — any machine authenticates, enrolls as a worker (declaring its specs), stays connected via SSE, and is eligible for channel dispatch. Workers pull the agent definition from the catalog when dispatched and run it locally.

- **Agent catalog as sandbox registry** — the existing `PublishVersion` / `PullVersion` endpoints serve as the push/pull mechanism for sandbox definitions. A machine creates a sandbox locally and publishes it as an agent version. Other workers pull that version when dispatched to process a channel's events.

- **Channel-granularity bus topics** — new topic namespace `channel.<provider>.<resource_id>.<event_type>` for per-resource event delivery, replacing `runner.<profile>.dispatch` as the primary routing mechanism.

## Capabilities

### New Capabilities
- `channel-routing`: resource-addressed event routing from any provider to per-resource worker sessions; channel key computation, session lookup, auto-provisioning, and event delivery
- `runner-spec-registration`: worker enrollment with declared agent specs; spec-aware worker selection and load-balanced assignment across any online worker
- `session-channel-binding`: durable binding of channel keys to active worker sessions; DB-backed registry with in-memory cache; channel lifecycle management (bind, unbind, cleanup)

### Modified Capabilities
- `agent-catalog`: catalog serves as the sandbox registry; `Dispatch` endpoint extended to support channel-based routing (not just named target); pull model documented as the worker's mechanism to fetch sandbox definitions
- `message-bus`: extend topic namespace to support `channel.<provider>.<resource_id>.<event_type>` pattern; add channel-scoped subscription filters for workers
- `daemon-runtime`: worker subscribes to per-channel topics instead of profile-scoped topics; enrolls with declared specs; pulls agent definition from catalog on dispatch
- `webhook-pipeline`: webhook handler publishes to channel router instead of directly to `runner.<profile>.dispatch` topic; adapter output normalized to `(provider_code, resource_id, spec)` tuple
- `rest-writeback`: write-back triggered by channel session context rather than by separate orchestrator path
- `provider-gateway`: adapter interface extended to expose a `ChannelKey(event) -> (provider, resourceID, spec)` method for provider-agnostic routing

## Impact

- **New packages**: `libs/server/channel/` (router, registry, provisioner)
- **Schema changes**: `runtimes` table gains `specs TEXT[]` column; new `channel_sessions` table for durable channel→session bindings
- **API changes**: worker enrollment body extended with `specs` field; new channel status endpoint for observability; `Dispatch` endpoint enhanced for channel-based routing
- **Dependency changes**: channel router depends on session manager, worker registry, agent catalog, and message bus
- **Migration**: existing runners re-enroll as workers or update their spec registration; profile-scoped topics deprecated but not removed
