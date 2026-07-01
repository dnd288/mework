# MeWork E2E Scenario Suite (interface-first)

This directory is the **end-to-end test design** for MeWork, written **before
implementation** — true BDD/TDD "evaluate the interface first." Each scenario describes
a **complete system setup** (the whole world: hub + runner + fake provider + database)
exercising one **end-user usage or API surface**, expressed as GIVEN/WHEN/THEN.

These are **specifications**; the runnable Go twin lives alongside them (see below).

## Go BDD suite (the executable twin)

The markdown scenarios are mirrored as **real, compiling Go BDD tests** in this same
directory (`package e2e`, all `*_test.go`):

- **`bdd_test.go`** — a dependency-free `Given/When/Then` harness. `Scenario(t, id, title,
  status).Given(...).When(...).Then(...).Run()`. `Run()` currently `t.Skip`s every
  scenario (the agent-hub target is not built yet); dropping that one Skip turns the whole
  catalog into live tests with no rewrite.
- **`api_test.go`** — the **proposed API under review**: the agent-hub DTOs and capability
  interfaces (`Broker`, `Catalog`, `Registry`, `Authenticator`, `GrantVerifier`,
  `SandboxDriver`, `SandboxManager`, `AgentBackend`, `Runner`) plus the `World` harness.
  Test-only — **no production code exists**; reading these shapes + the scenario bodies is
  the review.
- **`NN_*_test.go`** — one file per surface; each markdown scenario is one Go test that
  reads as `GIVEN/WHEN/THEN` and skips with its `[ID] … pending cNNNN` reason.

Run it (any Go ≥ 1.25 toolchain):

```bash
go test ./tests/e2e/ -v      # every scenario prints SKIP: [ID] … — pending cNNNN
```

`make test` stays green (all skipped); `go build ./...` ignores the test-only package.
The Go suite adds target surfaces beyond the markdown: tenant management (`TENANT-*`),
grant integrity (`GRANT-*`), smart/lazy subscriptions + push-to-sandbox (`BUS-12..16`),
agent backends claude/codex (`AGENT-*`), sandbox crash handling (`CRASH-*`), and
concurrency (`CONC-*`); plus the real-world platform surfaces scheduling (`SCHED-*`),
sessions (`SESSION-*`), interactive chat (`CHAT-*`), live status/streaming (`STREAM-*`,
`STATUS-*`), cancellation (`CANCEL-*`), quotas/audit (`QUOTA-*`, `AUDIT-*`),
notifications/artifacts (`NOTIFY-*`, `ARTIFACT-*`), runner selection/secrets
(`SELECT-*`, `SECRET-*`), and session workspaces backed by S3-compatible storage —
object store (`STORE-*`), workspace attach/sync (`WS-*`), shared-read + scoped-push
(`SHARE-*`), and workspace base code + lifecycle hooks (`WSHOOK-*`). **204 scenarios
total** (see [SCENARIOS.md](SCENARIOS.md) for the real-world coverage map).

These are also the direct input to `/opsx:ship-plan`'s test tasks: each scenario id is the
contract a real implementation must satisfy.

## How to read it

1. **[harness.md](harness.md)** — *the World*. The reusable complete system setup every
   scenario's `Background` composes (`Hub`, `DB`, `FakeProvider`, `FakeRunner`,
   `FakeCLI`, fixtures), plus the planned Go harness API. Read this first.
2. **The Go BDD suite** (`*_test.go` in this directory) — one file per usage/API surface;
   each scenario reads as GIVEN/WHEN/THEN and skips. Start with `api_test.go` (the proposed
   API), then any `NN_*_test.go`.
3. **[SCENARIOS.md](SCENARIOS.md)** — the master index: every scenario id → title →
   status → source spec → doc, plus coverage boundaries.

## Status badges

Every scenario is badged so the suite stays honest about what is real:

- **`[Implemented]`** — the behavior exists in today's code; the scenario is runnable
  (green-able) now against the current poll/queue pipeline.
- **`[Planned — cNNNN]`** — the behavior is specified in `openspec/changes/cNNNN-*` but
  not built; the scenario is **red** until that change lands (the interface-first part).

This mirrors `docs/`, which presents the agent-hub target architecture as canonical with
the same badges.

## Surfaces

| # | Go test file | What it evaluates |
|---|--------------|-------------------|
| 01 | `01_server_health_test.go` | startup secrets, migrations, `/healthz` |
| 02 | `02_auth_grants_test.go` | PAT, rt_token, runner identity, grants |
| 03 | `03_cli_onboarding_test.go` | login, config, provider/runtime/profile |
| 04 | `04_runner_enroll_test.go` | tenant management, `runner enroll`, durable identity |
| 05 | `05_daemon_test.go` | start/stop/status, backend detection, stdin, timeout |
| 06 | `06_webhook_intake_test.go` | signature, trigger grammar, idempotency, provider gateway |
| 07 | `07_jobs_poll_test.go` | claim/ack/heartbeat/state machine (today) |
| 08 | `08_message_bus_test.go` | SSE subscribe/publish/resume/ack, smart/lazy subs, push-to-sandbox |
| 09 | `09_agent_catalog_test.go` | publish/pull/dispatch/grant |
| 10 | `10_runner_loop_test.go` | enroll→subscribe→pull→run→report→ack, resume, crash recovery |
| 11 | `11_sandbox_test.go` + `11b_agents_test.go` | driver lifecycle, local/docker, limits, crash; claude/codex |
| 12 | `12_rest_writeback_test.go` | durable outbox, exactly-once |
| 13 | `13_journeys_test.go` | full end-to-end journeys (today + target) |
| 14 | `14_concurrency_test.go` | concurrent dispatch, one-active, isolation, ordering |
| 15 | `15_scheduling_test.go` | cron/interval/at, recurring, pause/cancel, missed-fire, timezone |
| 16 | `16_sessions_test.go` | session create/attach/list/close, resume, idle timeout, ownership |
| 17 | `17_chat_test.go` | interactive multi-turn chat: send→stream, history, cancel, isolation |
| 18 | `18_status_streaming_test.go` | agent→hub progress/log/output streaming, run status, presence |
| 19 | `19_cancellation_test.go` | cancel run (graceful→forced), propagate to sandbox, scheduled, idempotent |
| 20 | `20_quotas_audit_test.go` | per-tenant quotas/rate limits, audit log |
| 21 | `21_notify_artifacts_test.go` | outbound notifications/webhooks, artifact store/retrieve |
| 22 | `22_selection_secrets_test.go` | runner load-balancing/affinity, grant-scoped secret injection |
| 23 | `23_workspace_storage_test.go` | S3-compatible object store, session workspace attach/sync, shared-read + scoped-push |
| 24 | `24_workspace_hooks_test.go` | workspace base code + lifecycle hooks (clone git, setup, pre/post-run) |

## How this becomes Go tests

The suite is buildable with **zero new test frameworks**, reusing the patterns already in
`internal/integration/pipeline_test.go`:

- Real `mework-server` via `httptest.NewServer(server.NewServer(pool, cfg))`.
- Real Postgres via `TEST_DATABASE_URL` (the suite **skips** when it is unset); `-p 1`
  serialized because the DB is shared and truncated between scenarios.
- A fake Mello provider as a second `httptest.NewServer` (injected via `cfg.MelloBaseURL`).
- A `t.TempDir()` `FakeCLI` for AI-backend execution (the gap the current E2E skips).
- Plain `if got != want { t.Errorf(...) }` assertions — no testify.

When implementing, build `tests/e2e/harness.go` to the API sketched in
[harness.md](harness.md), then add `tests/e2e/*_test.go` translating each feature file —
`[Implemented]` scenarios go green immediately; `[Planned]` scenarios stay red (or
`t.Skip("pending cNNNN")`) until their change lands.

## Traceability

Every scenario cites its source OpenSpec scenario (`openspec/specs/*` baseline or
`openspec/changes/cNNNN/specs/*` delta) and the `docs/` section it evaluates, so the
three layers — **docs ↔ spec ↔ test** — stay in lockstep. See
[SCENARIOS.md](SCENARIOS.md) for the full map and the coverage boundaries (what is
intentionally out of scope, e.g. the removed claim/heartbeat requirements and the
structural c0001 change).
