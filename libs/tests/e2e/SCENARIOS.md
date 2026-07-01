# Scenario Index

Master catalog of every E2E scenario, its surface, status, and traceability back to the
source OpenSpec scenario and the doc it evaluates. Read a full scenario in its
`NN_*_test.go` file (the Go BDD suite); the World it runs in is [harness.md](harness.md).

- **Status** — `[Impl]` = `[Implemented]` (runnable against today's code);
  `[Pl·cNNNN]` = `[Planned]`, red until that change lands.
- **Spec** — `openspec/specs/<cap>` (baseline) or `openspec/changes/<cNNNN>` (delta).

## Coverage summary

| Surface | File | Scenarios | Status |
|---------|------|-----------|--------|
| Server & health | `01-server-and-health` | HEALTH-01..05 | Implemented |
| Auth & grants | `02-auth-and-grants` | AUTH-01..08 | Impl + c0003 |
| CLI onboarding | `03-cli-onboarding` | CLI-01..08 | Implemented |
| Runner enroll | `04-runner-enroll` | ENROLL-01..05 | c0005 |
| Daemon lifecycle & execution | `05-daemon-lifecycle` | DAEMON-01..10 | Implemented |
| Webhook intake & provider gateway | `06-webhook-intake` | HOOK-01..12 | Implemented |
| Job queue (poll) | `07-jobs-poll` | JOB-01..09 | Implemented |
| Message bus (SSE) | `08-message-bus-sse` | BUS-01..11 | c0003 |
| Agent catalog | `09-agent-catalog` | CAT-01..10 | c0004 |
| Runner loop | `10-runner-loop` | LOOP-01..07 | c0005 |
| Sandbox execution | `11-sandbox-execution` | SBX-01..09 | c0006 |
| REST write-back | `12-rest-writeback` | WB-01..04 | Implemented |
| Journeys | `13-journeys` | E2E-01..04 | Impl + target |

## Full index

| ID | Title | Status | Spec source | Doc |
|----|-------|--------|-------------|-----|
| HEALTH-01 | Missing required secret aborts startup | [Impl] | auth-and-secrets · Required server secrets | auth-and-secrets.md |
| HEALTH-02 | Each required secret is enforced | [Impl] | auth-and-secrets · Required server secrets | auth-and-secrets.md |
| HEALTH-03 | Migrations run on boot | [Impl] | provider-gateway · Provider-agnostic data model | deployment-guide.md |
| HEALTH-04 | Healthz OK when DB reachable | [Impl] | (operational) | api-reference.md |
| HEALTH-05 | Healthz 503 when DB down | [Impl] | (operational) | api-reference.md |
| AUTH-01 | PAT required for management routes | [Impl] | auth-and-secrets · Two-token authentication | auth-and-secrets.md |
| AUTH-02 | rt_token required for job routes | [Impl] | auth-and-secrets · Two-token authentication | auth-and-secrets.md |
| AUTH-03 | Runtime token shown once, stored hashed | [Impl] | auth-and-secrets · Runtime token generation and lookup | auth-and-secrets.md |
| AUTH-04 | Authenticate by lookup hash | [Impl] | auth-and-secrets · Runtime token generation and lookup | auth-and-secrets.md |
| AUTH-05 | Stored credential encrypted at rest | [Impl] | auth-and-secrets · Credential sealing at rest | auth-and-secrets.md |
| AUTH-06 | Unseal only for write-back | [Impl] | auth-and-secrets · Credential sealing at rest | auth-and-secrets.md |
| AUTH-07 | Runner credential required for transport routes | [Pl·c0004] | c0003 auth (MODIFIED) · Two-token authentication | auth-and-secrets.md |
| AUTH-08 | Grant scopes the operation, not just identity | [Pl·c0004] | c0003 auth (MODIFIED) · Two-token authentication | auth-and-secrets.md |
| CLI-01 | Log in with a Mello PAT | [Impl] | cli · Command surface | cli-and-usage.md |
| CLI-02 | Token file restrictive permissions | [Impl] | cli · Credential file safety | auth-and-secrets.md |
| CLI-03 | Config precedence flag > env > file | [Impl] | cli · Configuration resolution | cli-and-usage.md |
| CLI-04 | Profile isolation | [Impl] | cli · Configuration resolution | cli-and-usage.md |
| CLI-05 | Connect a provider | [Impl] | provider-gateway · Provider connections | cli-and-usage.md |
| CLI-06 | Register a runtime returns one-time token | [Impl] | cli · Command surface | cli-and-usage.md |
| CLI-07 | Create & manage an AI profile | [Impl] | cli · Command surface | cli-and-usage.md |
| CLI-08 | Machine-readable `--json` output | [Impl] | cli · Command surface | cli-and-usage.md |
| ENROLL-01 | Enroll a new runner | [Pl·c0005] | agent-runner · Install-once enrollment | auth-and-secrets.md |
| ENROLL-02 | Unattended after enrollment | [Pl·c0005] | agent-runner · Install-once enrollment | runtime-and-sandbox.md |
| ENROLL-03 | Invalid/expired registration token rejected | [Pl·c0005] | agent-runner · Install-once enrollment | auth-and-secrets.md |
| ENROLL-04 | Registration token not reusable as identity | [Pl·c0005] | agent-runner · Install-once enrollment | auth-and-secrets.md |
| ENROLL-05 | Inspect dispatched agents and active sessions | [Pl·c0005] | c0004 cli (MODIFIED) · Command surface | cli-and-usage.md |
| DAEMON-01 | Start in the background | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-02 | Inspect a running daemon | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-03 | Stop gracefully with SIGTERM fallback | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-04 | Restart | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-05 | Follow logs | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-06 | Stale pid not mistaken for live | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-07 | Per-profile health port deterministic | [Impl] | daemon-runtime · Daemon lifecycle management | runtime-and-sandbox.md |
| DAEMON-08 | Fall back to the next AI backend | [Impl] | daemon-runtime · AI backend detection | runtime-and-sandbox.md |
| DAEMON-09 | Prompt fed on stdin, never argv (today) | [Impl] | daemon-runtime · Safe, isolated execution | philosophy.md |
| DAEMON-10 | Runaway run is bounded (today) | [Impl] | daemon-runtime · Safe, isolated execution | runtime-and-sandbox.md |
| HOOK-01 | Reject unsigned/mis-signed payload | [Impl] | webhook-pipeline · Webhook ingestion endpoint | api-reference.md |
| HOOK-02 | Accept valid signed payload | [Impl] | webhook-pipeline · Webhook ingestion endpoint | api-reference.md |
| HOOK-03 | Parse profile and workflow | [Impl] | webhook-pipeline · Trigger grammar | api-reference.md |
| HOOK-04 | Parse profile only | [Impl] | webhook-pipeline · Trigger grammar | api-reference.md |
| HOOK-05 | Workflow keyword normalized to canonical case | [Impl] | webhook-pipeline · Trigger grammar | api-reference.md |
| HOOK-06 | Not a trigger inside another token | [Impl] | webhook-pipeline · Trigger grammar | api-reference.md |
| HOOK-07 | Skip the runner's own comment | [Impl] | webhook-pipeline · Self-retrigger guard | philosophy.md |
| HOOK-08 | Idempotent on duplicate delivery | [Impl] | webhook-pipeline · Idempotent enqueue | philosophy.md |
| HOOK-09 | Instructions are capped | [Impl] | webhook-pipeline · Trigger grammar | api-reference.md |
| HOOK-10 | Resolve a registered provider adapter | [Impl] | provider-gateway · Provider adapter registry | architecture.md |
| HOOK-11 | Reject an unknown provider | [Impl] | provider-gateway · Provider adapter registry | api-reference.md |
| HOOK-12 | Add a new provider without a migration | [Impl] | provider-gateway · Provider-agnostic data model | philosophy.md |
| JOB-01 | Claim leases the oldest queued job | [Impl] | job-queue · Transactional state machine | api-reference.md |
| JOB-02 | Concurrent claims don't double-assign | [Impl] | job-queue · Exactly-once claim | architecture.md |
| JOB-03 | One active job per runtime | [Impl] | job-queue · Exactly-once claim | philosophy.md |
| JOB-04 | Ack running then done | [Impl] | daemon-runtime · Stateless poll worker | api-reference.md |
| JOB-05 | Reject transition out of terminal state | [Impl] | job-queue · Transactional state machine | philosophy.md |
| JOB-06 | Idempotent re-ack | [Impl] | job-queue · Transactional state machine | api-reference.md |
| JOB-07 | Ownership is enforced | [Impl] | daemon-runtime · Stateless poll worker | api-reference.md |
| JOB-08 | Heartbeat extends the lease | [Impl] | job-queue · Heartbeat and lease | api-reference.md |
| JOB-09 | Sweeper reclaims an abandoned job | [Impl] | job-queue · Lease sweeper | architecture.md |
| BUS-01 | Publish to a topic with subscribers | [Pl·c0003] | message-bus · Topic-based publish | api-reference.md |
| BUS-02 | Publish with no subscribers is retained | [Pl·c0003] | message-bus · Topic-based publish | architecture.md |
| BUS-03 | Receive a pushed event | [Pl·c0003] | message-bus · SSE subscription contract | api-reference.md |
| BUS-04 | Multiple topics on one stream | [Pl·c0003] | message-bus · SSE subscription contract | architecture.md |
| BUS-05 | Resume after a dropped connection | [Pl·c0003] | message-bus · Resumable delivery | architecture.md |
| BUS-06 | Ack marks a message handled | [Pl·c0003] | message-bus · Delivery acknowledgement | api-reference.md |
| BUS-07 | Unacked message is redeliverable | [Pl·c0003] | message-bus · Delivery acknowledgement | architecture.md |
| BUS-08 | Subscriber restricted to entitled topics | [Pl·c0003] | message-bus · SSE subscription contract | auth-and-secrets.md |
| BUS-09 | Swap backend without breaking clients | [Pl·c0003] | message-bus · Pluggable broker backend | architecture.md |
| BUS-10 | State tracked independently of transport | [Pl·c0003] | c0002 job-queue (MODIFIED) · Transactional state machine | architecture.md |
| BUS-11 | Distinct events publish distinct messages | [Pl·c0003] | c0002 webhook (MODIFIED) · Idempotent enqueue | architecture.md |
| CAT-01 | Publish a new agent version | [Pl·c0004] | agent-catalog · Versioned agent artifacts | api-reference.md |
| CAT-02 | Republishing an existing version rejected | [Pl·c0004] | agent-catalog · Versioned agent artifacts | api-reference.md |
| CAT-03 | Resolve a moving pointer (@latest) | [Pl·c0004] | agent-catalog · Versioned agent artifacts | api-reference.md |
| CAT-04 | Pull a definition-form agent | [Pl·c0004] | agent-catalog · Type-agnostic artifact form | api-reference.md |
| CAT-05 | Pull an image-form agent | [Pl·c0004] | agent-catalog · Type-agnostic artifact form | runtime-and-sandbox.md |
| CAT-06 | Authorized pull succeeds | [Pl·c0004] | agent-catalog · Pull an agent | api-reference.md |
| CAT-07 | Unauthorized pull is denied | [Pl·c0004] | agent-catalog · Pull an agent | auth-and-secrets.md |
| CAT-08 | Dispatch reaches the target runner | [Pl·c0004] | agent-catalog · Dispatch an agent to a target | api-reference.md |
| CAT-09 | Dispatch carries an explicit scoped grant | [Pl·c0004] | agent-catalog · Scoped permission grants | auth-and-secrets.md |
| CAT-10 | Absent grant denies a privileged operation | [Pl·c0004] | agent-catalog · Scoped permission grants | philosophy.md |
| LOOP-01 | Runner comes online (presence) | [Pl·c0005] | agent-runner · SSE subscription and presence | runtime-and-sandbox.md |
| LOOP-02 | No interval polling when idle | [Pl·c0005] | c0004 daemon (MODIFIED) · Stateless poll worker | runtime-and-sandbox.md |
| LOOP-03 | Receive a dispatch by push | [Pl·c0005] | agent-runner · SSE subscription and presence | architecture.md |
| LOOP-04 | Successful dispatch lifecycle | [Pl·c0005] | agent-runner · Pull-run-report loop | runtime-and-sandbox.md |
| LOOP-05 | Failed run reported, not dropped | [Pl·c0005] | agent-runner · Pull-run-report loop | runtime-and-sandbox.md |
| LOOP-06 | Operation within the grant proceeds | [Pl·c0005] | agent-runner · Grant enforcement on the client | philosophy.md |
| LOOP-07 | Operation outside the grant refused locally | [Pl·c0005] | agent-runner · Grant enforcement on the client | auth-and-secrets.md |
| SBX-01 | Run an agent through the driver interface | [Pl·c0006] | sandbox-runtime · Sandbox driver interface | runtime-and-sandbox.md |
| SBX-02 | Prompt never placed on the command line | [Pl·c0006] | sandbox-runtime · Sandbox driver interface | philosophy.md |
| SBX-03 | Select the local driver | [Pl·c0006] | sandbox-runtime · Selectable drivers | runtime-and-sandbox.md |
| SBX-04 | Select the docker driver | [Pl·c0006] | sandbox-runtime · Selectable drivers | runtime-and-sandbox.md |
| SBX-05 | Add a driver without changing callers | [Pl·c0006] | sandbox-runtime · Selectable drivers | runtime-and-sandbox.md |
| SBX-06 | Isolation between runs | [Pl·c0006] | sandbox-runtime · One agent per sandbox | runtime-and-sandbox.md |
| SBX-07 | Sandbox torn down after the run | [Pl·c0006] | sandbox-runtime · One agent per sandbox | runtime-and-sandbox.md |
| SBX-08 | Docker driver confines host paths | [Pl·c0006] | sandbox-runtime · Isolation and resource limits | runtime-and-sandbox.md |
| SBX-09 | Resource limit terminates a runaway agent | [Pl·c0006] | sandbox-runtime · Isolation and resource limits | runtime-and-sandbox.md |
| WB-01 | Post the result back to the provider | [Impl] | rest-writeback · Server-side REST write-back | api-reference.md |
| WB-02 | Runner holds no write-back credentials | [Impl] | rest-writeback · Server-side REST write-back | philosophy.md |
| WB-03 | Retry after a transient failure | [Impl] | rest-writeback · Durable outbox delivery | api-reference.md |
| WB-04 | No duplicate comment on restart | [Impl] | rest-writeback · Durable outbox delivery | philosophy.md |
| E2E-01 | Today: comment → webhook → claim → run → write-back | [Impl] | webhook-pipeline + job-queue + daemon-runtime + rest-writeback | cli-and-usage.md |
| E2E-02 | Target: publish → enroll → dispatch → sandbox → write-back | [Pl·target] | agent-catalog + agent-runner + sandbox-runtime + message-bus | architecture.md |
| E2E-03 | Target resilience: disconnect → resume → exactly-one write-back | [Pl·target] | message-bus · Resumable delivery + rest-writeback · Durable outbox | architecture.md |
| E2E-04 | Operator deploy → developer onboard → first run | [Impl] | cli + provider-gateway + webhook-pipeline | deployment-guide.md |

## Go BDD suite additions (target surfaces)

The runnable Go twin (see [README.md](README.md) → "Go BDD suite") adds these target-only
scenarios beyond the markdown above. All skip pending their change.

| ID | Title | Status | Surface |
|----|-------|--------|---------|
| BUS-12 | Smart subscription delivers only matching events | [Pl·c0003] | bus · smart/filtered subscription |
| BUS-13 | Non-matching events are not materialized (lazy) | [Pl·c0003] | bus · lazy delivery |
| BUS-14 | Push a control message down to a running sandbox | [Pl·c0003] | bus · push-to-sandbox |
| BUS-15 | Session control channels are isolated | [Pl·c0003] | bus · session control |
| BUS-16 | Slow subscriber does not stall the bus | [Pl·c0003] | bus · backpressure |
| TENANT-01 | Register an isolated tenant | [Pl·c0005] | registry · tenant management |
| TENANT-02 | Tenants are isolated from each other | [Pl·c0005] | registry · tenant isolation |
| TENANT-03 | Registration tokens are scoped to a tenant | [Pl·c0005] | registry · token scope |
| GRANT-01 | A tampered grant fails integrity verification | [Pl·c0004] | auth · grant integrity |
| GRANT-02 | Absent grant denies by default | [Pl·c0004] | auth · least-privilege |
| GRANT-03 | Grants are scoped per run, not per identity | [Pl·c0004] | auth · per-run scope |
| AGENT-01 | Backend detection order (claude→codex→opencode) | [Pl·c0006] | agent · detection |
| AGENT-02 | Claude Code backend runs an agent | [Pl·c0006] | agent · claude |
| AGENT-03 | Codex backend runs an agent | [Pl·c0006] | agent · codex |
| AGENT-04 | Backends receive the prompt over stdin | [Pl·c0006] | agent · stdin |
| AGENT-05 | No installed backend is handled | [Pl·c0006] | agent · unavailable |
| LOOP-08 | Reconnect with jittered backoff and resume | [Pl·c0005] | runner · resume |
| LOOP-09 | Runner restarts and recovers in-flight bookkeeping | [Pl·c0005] | runner · crash recovery |
| SBX-10 | Pull an image-form agent into a sandbox | [Pl·c0006] | sandbox · pull |
| SBX-11 | Inspect a running sandbox's state | [Pl·c0006] | sandbox · manage |
| CRASH-01 | A crashed sandbox is reported failed | [Pl·c0006] | sandbox · crash |
| CRASH-02 | Resources are released after a crash | [Pl·c0006] | sandbox · crash cleanup |
| CRASH-03 | Runner survives a sandbox crash and keeps serving | [Pl·c0006] | runner · crash isolation |
| CONC-01 | Concurrent dispatches to one runner are all delivered | [Pl·c0005] | concurrency · dispatch |
| CONC-02 | A runner runs one agent at a time | [Pl·c0005] | concurrency · one-active |
| CONC-03 | Concurrent sandboxes do not interfere | [Pl·c0006] | concurrency · isolation |
| CONC-04 | Per-topic delivery is ordered under concurrent publish | [Pl·c0003] | concurrency · ordering |
| CONC-05 | Concurrent sessions never cross-deliver | [Pl·c0003] | concurrency · isolation |
| E2E-05 | Multi-tenant concurrent journeys stay isolated | [Impl] | journey · multi-tenant |

## Real-world platform surfaces (Go suite, proposed — all skipped)

Added so the suite covers what a production agent hub needs beyond the core. Status
`[Pl·plat]` = proposed platform capability (no openspec change yet).

| ID range | Surface | File |
|----------|---------|------|
| SCHED-01..07 | Scheduling: cron/interval/at, recurring re-arm, pause/resume/cancel, missed-fire policy, timezone, per-tenant list | `15_scheduling_test.go` |
| SESSION-01..07 | Session lifecycle: create→attach→close, list/status/owner, resume-after-reconnect, multiple-per-runner, idle timeout, ownership, tenant isolation | `16_sessions_test.go` |
| CHAT-01..06 | Interactive chat: send→stream, multi-turn history, cancel mid-turn, concurrent isolation, system prompt, backpressure | `17_chat_test.go` |
| STREAM-01..05, STATUS-01..03 | Agent→hub comms: emit progress/log/output, client tails a run, late-subscriber tail, per-run ordering, output→write-back; run status transitions, presence detail, status overview | `18_status_streaming_test.go` |
| CANCEL-01..04 | Cancel running run (graceful→forced), propagate to sandbox, cancel scheduled, idempotent/terminal | `19_cancellation_test.go` |
| QUOTA-01..03, AUDIT-01..03 | Per-tenant concurrent/rate limits, queryable limits; security actions audited, queryable, ordered | `20_quotas_audit_test.go` |
| NOTIFY-01..03, ARTIFACT-01..04 | Completion/failure webhooks + retry; store/retrieve/list artifacts, checksum integrity | `21_notify_artifacts_test.go` |
| SELECT-01..03, SECRET-01..03 | Runner load-balancing, session affinity, no-eligible-runner; grant-scoped secret injection, never-in-argv/logs, scope enforcement | `22_selection_secrets_test.go` |

## Storage & workspaces (Go suite, proposed — all skipped)

Session/run-scoped workspaces backed by S3-compatible storage, with shared-read +
scoped-push and base-code/hooks. Three layers: `ObjectStore` → `WorkspaceManager` →
`WorkspaceFS`.

| ID range | Surface | File |
|----------|---------|------|
| STORE-01..07 | S3-compatible object store: put/get, list-by-prefix, head, delete, presigned GET/PUT, endpoint-agnostic (AWS/MinIO/R2), multipart | `23_workspace_storage_test.go` |
| WS-01..09 | Attach folder→rw mount, sandbox writes a real file, sync-to-remote, detach-flush, force-sync+status, re-attach restores from remote, per-session isolation, write-outside-prefix denied (traversal blocked), agent never holds store creds (presigned) | `23_workspace_storage_test.go` |
| SHARE-01..06 | Read across shared root, shared root read-only, push only the grant-allowed folder, push-outside denied, published folder readable by other sessions, grant scopes read(broad)/write/push(narrow) | `23_workspace_storage_test.go` |
| WSHOOK-01..08 | Base code + lifecycle hooks: init clones a git repo, setup installs deps, pre_run/post_run bracket the agent, failing init aborts the run, hooks run within grant scope, post_sync after remote push, base from archive/store template, hook scripts over stdin not argv | `24_workspace_hooks_test.go` |

## Real-world coverage map (each named concern → scenarios)

| Concern (as requested) | Covered by |
|------------------------|------------|
| daemon management | DAEMON-01..10 |
| auth | AUTH-01..08, GRANT-01..03 |
| status | STATUS-01..03, STREAM-*, LOOP-01, DAEMON-02 |
| pull / create sandboxes | SBX-06, SBX-07, SBX-10, SBX-11 |
| handle sandbox sessions | SESSION-01..07, BUS-14/15 |
| concurrency | CONC-01..05, CHAT-04, SESSION-04 |
| crash recovery | CRASH-01..03, LOOP-09 |
| push message to sandbox | BUS-14, CANCEL-02 |
| sandbox subscribe events | BUS-15, STREAM-01..04 |
| communication events | STREAM-01..05, BUS-01..16 |
| schedules | SCHED-01..07 |
| sessions | SESSION-01..07 |
| chatting | CHAT-01..06 |
| claude code / codex | AGENT-01..05 |
| (platform) quotas/audit/notify/artifacts/selection/secrets | QUOTA/AUDIT/NOTIFY/ARTIFACT/SELECT/SECRET |
| online storage / workspace / S3 | STORE-01..07 |
| attach folder to a session, sandbox writes, sync to remote | WS-01..09 |
| shared root read-all, push only allowed folder | SHARE-01..06 |
| workspace base code + run sandbox hooks (clone git, etc.) | WSHOOK-01..08 |

## Notes on coverage boundaries

- **Intentionally not covered (removed under the target bus):** the job-queue
  requirements *Exactly-once claim* and *Heartbeat and lease* are REMOVED by c0002 (work
  is pushed over SSE, not claimed). They are covered here only for **today's** baseline
  (JOB-02/03/08) and have no target-architecture counterpart.
- **MODIFIED-in-place requirements** appear under both a baseline scenario and a target
  scenario, matched by title **and** source path: *Two-token authentication*
  (AUTH-01..06 baseline / AUTH-07..08 c0003), *Stateless poll worker* (JOB-04 baseline /
  LOOP-02 c0004), *Safe isolated execution* (today's daemon / SBX-* c0005).
- **c0001-repo-restructure** is a structural/CI change (import-guard, per-component
  build, behavior-preservation). Its scenarios are verified by Go-level arch-lint and
  build tests, not by this system-level E2E suite, so they are intentionally out of
  scope here.
