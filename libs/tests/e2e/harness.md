# The World — E2E Harness Design

> The **complete system setup** every scenario's `GIVEN` composes. Each Go BDD scenario
> (the `*_test.go` files in this directory) is written as if the whole system is running;
> this file defines what "the whole system" means and the named building blocks scenarios
> reference. It is **interface-first**: it describes the harness a future
> `tests/e2e/harness.go` must expose, grounded in the seams that already exist in
> `internal/integration/pipeline_test.go` — so the scenarios drop straight into Go with
> **no new test framework** (no testify; white-box; `go test -p 1`).

## Why a shared World

The existing E2E test (`internal/integration/pipeline_test.go`) hand-rolls the whole
setup inline and **simulates the daemon by calling `meworkclient` directly** — it never
exercises the real poll loop or the AI CLI. These scenarios formalize that setup into a
reusable World and close the two gaps (a real runner loop, a real `FakeCLI`), so the
same harness serves both the **`[Implemented]`** baseline and the **`[Planned]`**
agent-hub target.

## Building blocks

Every `Background` is assembled from these named components. Names in `code` are the
handles scenarios use.

### `DB` — Postgres `[Implemented]`
- Real Postgres via `TEST_DATABASE_URL`; the suite **skips** when it is unset
  (`t.Skip`), matching every DB-backed test in the repo.
- `store.RunMigrations(dsn)` on setup; `store.RollbackMigrations(dsn)` deferred.
- Truncate between scenarios in FK-safe order:
  `DELETE FROM jobs; watched_containers; account_identities; runtimes; profiles; provider_connections; accounts;`
  (target adds `agent_versions; agents;`).
- Because the DB is shared and truncated globally, the suite runs **serialized**
  (`-p 1`); scenarios that mutate shared state run in order, not as parallel `t.Run`
  subtests (the repo convention for DB tests).

### `Hub` — the mework-server `[Implemented]` → agent hub `[Planned]`
- The real router: `srv := server.NewServer(pool, cfg)` wrapped in
  `httptest.NewServer(srv)`.
- `cfg` (`server.Config`) fields used: `DatabaseURL`, `ListenAddr: "127.0.0.1:0"`,
  `WebhookSecret`, `ServerKey`, `MeworkSecretKey`, `MelloBaseURL` (points at
  `FakeProvider`). Required-secret scenarios construct `cfg` with one field blank to
  assert fail-fast.
- Target adds the SSE bus + catalog routes on the same router.

### `FakeProvider` — fake Mello REST `[Implemented]`
- One `httptest.NewServer` multiplexing by method+path, exactly as `pipeline_test.go`:
  - `GET /me` — PAT auth; returns a `mello.User`; increments `meCallCount`.
  - `GET /tickets/{id}` — provider-token auth; returns a `mello.TicketDetail`;
    increments `ticketCallCount`.
  - `POST /tickets/{id}/comments` — write-back; records `lastCommentBody`; increments
    `writebackCallCount`; can be toggled to return `5xx` to drive retry scenarios.
- Injected via `cfg.MelloBaseURL`; the adapter is wired with
  `provider.Register(melloprovider.NewMelloAdapter(FakeProvider.URL))`.
- The `provider.Provider` interface (`Code/ExtractContainerID/VerifyWebhook/ParseEvent/WriteBack`)
  is the seam for adding a second fake provider to prove provider-agnosticism.

### `FakeRunner` — the client side
- **Today `[Implemented]`** — drives the job API through the typed
  `meworkclient.Client` (`Claim` / `Ack` / `Heartbeat`), the way `pipeline_test.go`
  stands in for the daemon.
- **Target `[Planned — c0005]`** — an SSE consumer that holds a `text/event-stream`
  connection, reads events with monotonic `id`, reconnects with `Last-Event-ID`, POSTs
  acks out-of-band, and pulls dispatched agent versions. Authenticated with a runner
  identity credential, not `rt_token`.

### `FakeCLI` — the AI backend `[Implemented for unit, new for E2E]`
- A script written to `t.TempDir()`, `chmod 0755`, placed on `PATH` via `t.Setenv`,
  standing in for `claude`/`codex`/`opencode` — the pattern `agentrun/detect_test.go`
  and `runner_test.go` already use (`/bin/cat` proves stdin; `/bin/false` proves exit
  codes).
- Configurable modes: **echo-stdin** (assert the prompt arrived on stdin, never argv),
  **fixed-exit** (drive `done` vs `failed`), **sleep** (trip the 30-min/limit timeout).

### Fixtures — seeded state
Helpers seed a coherent account graph (mirroring `pipeline_test.go` steps A):
- `account` + `account_identities` (provisioned by a first PAT-authed call).
- `runtime` (today, via `CreateRuntime` → one-time `rt_token`) or **runner identity**
  (target, via enroll).
- sealed `provider_connection` (via `CreateConnection`, AES-sealed at rest).
- `profile` (today, via `CreateProfile`) or **agent version** (target, via catalog
  publish).
- `watched_containers` row mapping the board to the account (raw `INSERT`).
- **HMAC webhook signer**: `sig = "sha256=" + hex(HMAC-SHA256(secret, ts + "." + body))`
  with headers `X-Mello-Signature/-Timestamp/-Delivery-Id` — the exact construction the
  handler verifies.

## Planned Go harness API (interface-first)

The contract a future `tests/e2e/harness.go` should expose so each scenario becomes a
few lines. Described here, **not yet implemented**:

```go
// Setup — skips if TEST_DATABASE_URL unset; migrates, truncates, starts Hub + FakeProvider.
w := e2e.NewWorld(t)                 // returns *World, registers t.Cleanup for teardown
w.Config(func(c *server.Config){ ... }) // mutate cfg before Hub starts (e.g. blank a secret)

// Fixtures
acc   := w.SeedAccount("user-pat-token")
conn  := w.ConnectProvider(acc, "mello", melloToken, webhookSecret)
prof  := w.CreateProfile(acc, "dev", "system prompt", "claude", "claude-code")
w.WatchContainer(acc, "mello", "board-789")
rt    := w.RegisterRuntime(acc, "dev", "Dev Machine")     // today → rt_token
runner:= w.EnrollRunner(acc, "macbook")                   // target → runner identity

// Inbound
w.PostWebhook(WebhookOpts{Board:"board-789", Ticket:"tkt-999", Comment:"@mework dev review fix the bug", DeliveryID:"d1"})

// Client side (today)
job := w.Claim(rt); w.Ack(rt, job, "running"); w.RunFakeCLI(job, CLIEcho); w.Ack(rt, job, "done", summary)

// Client side (target)
ev  := w.Bus.Subscribe(runner, []string{"runner.macbook.dispatch"}, lastID)  // SSE stream
ver := w.Catalog.PublishVersion("code-fixer", "1.2.0", DefinitionForm, manifest)
w.Catalog.Dispatch("code-fixer@1.2.0", runner, grant)
w.RunInSandbox(ev, DriverLocal)         // or DriverDocker

// Assertions (plain if/t.Errorf — no assert lib)
w.AssertJobStatus(job, "queued")
w.AssertWriteback(1, "mework dev review — done", summary) // count + substrings on FakeProvider
w.AssertWritebackStatus(job, "success")
w.AssertDelivered(ev, msgID); w.AssertNotRedelivered(ev, msgID)
w.AssertDenied(op)                       // grant enforcement
```

Naming maps 1:1 to the GIVEN/WHEN/THEN verbs in the feature files, so a scenario's prose
translates directly to harness calls.

## Timing & async notes
- The webhook handler fetches the ticket snapshot **asynchronously**; write-back runs
  **asynchronously** after a terminal ack. `pipeline_test.go` settles these with a
  `time.Sleep(200ms)` + an SQL poll. The harness should prefer an **eventually(cond,
  timeout)** poll helper over fixed sleeps, but the behavior under test is the same.
- Target SSE scenarios assert **push latency is bounded without polling** — the runner
  receives a dispatch over the open stream with no intervening claim request.

## Traceability
Every scenario cites the source spec (`openspec/specs/*` or
`openspec/changes/cNNNN/specs/*`) and the doc section it evaluates
(`docs/api-reference.md`, `docs/cli-and-usage.md`, `docs/runtime-and-sandbox.md`,
`docs/auth-and-secrets.md`). The full map is in [SCENARIOS.md](SCENARIOS.md).
