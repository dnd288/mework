## 1. Database Schema

- [ ] 1.1 Add migration: `runtimes` table gains `specs TEXT[]` column (nullable)
- [ ] 1.2 Add migration: create `channel_sessions` table with `channel_key`, `session_id`, `provider_code`, `resource_id`, `runner_id`, `spec`, `status`, `created_at`, `closed_at`
- [ ] 1.3 Add indexes on `channel_sessions`: by `runner_id`, by `(provider_code, resource_id)`

## 2. Runner/Worker Spec Registration

- [ ] 2.1 Update `Runtime` struct and `Service` to support `specs []string`
- [ ] 2.2 Update worker enrollment endpoint to accept and validate `specs` field against agent catalog
- [ ] 2.3 Update heartbeat handler to update `runtimes.specs` from heartbeat payload
- [ ] 2.4 Update `RunnerInfo` and `RunnerSelectorImpl` to include `Specs` field
- [ ] 2.5 Implement spec-filtered worker selection: `SelectWorker(spec string) → runnerID` picks any online worker matching the spec, load-balanced by fewest active channels

## 3. Channel Registry

- [ ] 3.1 Create `ChannelRegistry` interface and `PostgresChannelRegistry` implementation
- [ ] 3.2 Implement `Bind(channelKey, sessionID, runnerID)` with advisory lock to prevent double-provisioning
- [ ] 3.3 Implement `Unbind(channelKey)`, `Lookup(channelKey) → sessionID`
- [ ] 3.4 Add in-memory `sync.Map` cache with DB fallback on miss
- [ ] 3.5 Populate cache from DB on server startup
- [ ] 3.6 Implement `RunnerActiveChannelCount(runnerID) → int` for load-balanced assignment

## 4. Channel Router

- [x] 4.1 Create `channel.Router` struct with `Route(ctx, providerCode, resourceID, eventPayload) error`
- [x] 4.2 Implement event → channel key computation (`providerCode:resourceID`)
- [x] 4.3 Implement lookup: check cache, fall back to DB, if no session → call AutoProvisioner
- [x] 4.4 Implement event publishing to channel bus topic: `channel.<provider>.<resourceID>.<eventType>`
- [x] 4.5 Add channel lifecycle transitions: `active → draining → closed`
- [x] 4.6 Add feature flag to toggle between old and new routing paths

## 5. Sandbox Bundle Format

- [x] 5.1 Define the bundle folder structure and `sandbox.yaml` metadata schema
- [x] 5.2 Add `"bundle"` as a recognized form in the agent catalog's `PublishVersion` validation
- [x] 5.3 Implement bundle validation on publish: require `sandbox.yaml`, `definition.md` inside zip, reject malformed bundles
- [x] 5.4 Implement bundle extraction on the worker side: `PullVersion` returns zip → worker extracts to isolated workdir
- [x] 5.5 Implement bundle materialization: read `sandbox.yaml` → load `definition.md` as prompt → mount `tools/` → register hooks from `hooks/`
- [x] 5.6 Add CLI command: `mework sandbox pack <dir> --output sandbox.zip` to create a bundle from a local folder
- [x] 5.7 Add CLI command: `mework sandbox push <name> --version <ver> --file sandbox.zip` to publish a bundle to the catalog
- [x] 5.8 Add validation: `sandbox.yaml` must declare a valid `spec` and `backend`; reject publish if missing or invalid

## 6. Auto-Provisioner (Agent Catalog Integration)

- [x] 6.1 Create `AutoProvisioner` struct wired to session manager, worker selector, agent catalog, and message bus
- [x] 6.2 Implement spec derivation from resolved profile's `backend_hint` → agent catalog name
- [x] 6.3 Implement spec-aware `SelectWorker(spec)` using spec-filtered runner query
- [x] 6.4 Implement session creation via `session.Manager.Create()`
- [x] 6.5 Implement channel binding (write to `channel_sessions`, update cache)
- [x] 6.6 Implement agent catalog dispatch: call `Dispatch` to send agent to selected worker with scoped grant
- [x] 6.7 Implement sandbox spawn: worker pulls agent via `PullVersion` (zip if bundle form), extracts, subscribes to channel topic, runs locally
- [x] 6.8 Implement retry with backoff (3×, 5s apart) when no eligible worker is online

## 6. Bus Topic Extension

- [x] 6.1 Verify `MatchTopic` supports `channel.<provider>.<id>.*` pattern
- [x] 6.2 Add tests for channel-scoped `MatchTopic` patterns
- [x] 6.3 Add tests for channel isolation: two channels on same worker don't leak events

## 7. Webhook Handler Integration

- [x] 7.1 Extend provider adapter interface with `ChannelKey(rawPayload) → (providerCode, resourceID)`
- [x] 7.2 Implement `ChannelKey` on the Mello adapter
- [x] 7.3 Wire `ChannelRouter.Route()` into webhook handler after trigger parsing (alongside existing path initially)
- [x] 7.4 Add feature flag toggle: old path (profile-topic) vs new path (channel routing)

## 8. Daemon (Worker) Integration

- [x] 8.1 Update daemon enrollment to detect and declare installed AI CLIs as specs (e.g., `claude` in PATH → `"claude-code"`)
- [x] 8.2 Update daemon heartbeat to include current specs
- [x] 8.3 Implement agent pull: when dispatched, worker calls `PullVersion` with scoped grant to fetch sandbox definition
- [x] 8.4 Implement channel subscription: sandbox subscribes to `channel.<provider>.<id>.*` bus topic
- [x] 8.5 Add backward-compatible enrollment: no specs → treated as capable of all specs

## 9. Write-back via Channel Session

- [x] 9.1 Implement write-back lookup from channel session context (accountID + providerCode)
- [x] 9.2 Wire channel session-based write-back into result processing pipeline (instead of separate job lookup)
- [x] 9.3 Add test: write-back uses session context, worker never holds provider token

## 10. Observability and Cleanup

- [ ] 10.1 Implement `GET /api/v1/channels` endpoint for listing active channel sessions
- [ ] 10.2 Create channel session sweeper (30s interval, close orphaned/closed sessions)
- [ ] 10.3 Add structured logging throughout router, registry, and provisioner
- [ ] 10.4 Add metrics: active channel count, provision latency, routing latency

## 11. E2E Tests

- [ ] 11.1 Add test: webhook triggers channel routing → auto-provision → session created → event delivered to worker
- [ ] 11.2 Add test: spec-filtered worker selection picks correct worker by declared specs
- [ ] 11.3 Add test: no eligible worker → event buffered → retry with backoff
- [ ] 11.4 Add test: channel lifecycle active → draining → closed
- [ ] 11.5 Add test: orphaned channel session reaped by sweeper when worker goes offline
- [ ] 11.6 Add test: agent publish → dispatch → pull flow (push sandbox, other worker pulls)
- [ ] 11.7 Update `examples/mello-claude` integration test to exercise channel routing path
