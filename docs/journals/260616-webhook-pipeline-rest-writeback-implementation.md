# Webhook Pipeline, REST Write-Back, and Daemon Reshape

**Date**: 2026-06-16 11:30
**Severity**: High
**Component**: Webhook, Agent Runtime Daemon, Job Queueing, Write-back
**Status**: Resolved

## What Happened

We completed a major architectural simplification by replacing the client-side Model Context Protocol (MCP) write-back model with a server-side webhook and direct REST API write-back pipeline. The agent-runtime daemon was reshaped into a stateless poll-based worker, purging local state tracking entirely.

Key milestones of this implementation include:
1. **Direct REST API Write-back**: Replaced the client-side MCP write-back client (`internal/mcp`) with a direct server-side write-back execution calling the Mello REST API `CreateComment` endpoint.
2. **Server-Side Trigger Parsing**: Added a server-side webhook handler (`internal/server/webhook/handler.go`) that extracts target container ids, validates actor permissions against `account_identities`, and parses trigger grammar comments in the format `@mework [profile] [workflow] [instructions]` via `ParseTrigger` to enqueue jobs.
3. **Runtime Token Authentication**: Secured jobs claim, ack, and heartbeat API endpoints using secure Bearer runtime tokens (`rt_token`), which lookup runtime details via hashed token signatures (`token_lookup`) and dynamically update daemon status to `online` and refresh `last_seen_at`.
4. **Concurrency and Lock Control**: Implemented postgres transaction isolation for job claims via `pg_advisory_xact_lock(hashtext($1))` scoped to the `runtime_id`, preventing race conditions where a runtime could double-claim jobs, and used `SELECT ... FOR UPDATE SKIP LOCKED` to safely claim the oldest queued job in a concurrent-safe queue.
5. **Daemon Reshape**: Cleaned up the daemon's runtime footprint by removing the `internal/mcp` directory and the local `state.json` file. The daemon is now entirely stateless, relying only on runtime config and the server REST API for its task lifecycle.
6. **E2E Integration Success**: Verified the entire flow—from mock Mello signature verified webhook, job queueing, runtime claim/ack/heartbeat, through to final mock REST API comment write-back—with a green E2E test `TestFullPipelineE2E` in `internal/integration/pipeline_test.go`.

## The Brutal Truth

Managing state in two places is a recipe for disaster. The previous architecture, which relied on a client-side MCP bridge running on the local daemon machine and a local `state.json` file tracking executed comment IDs, was a distributed-systems nightmare. It was extremely fragile: if a daemon crashed, `state.json` became corrupt or went out of sync, leading to double-execution loops or lost runs. Worse, the local daemon had to know the personal access tokens of the user to make write-backs directly via MCP, which meant distributing credentials everywhere. 

Purging the entire `internal/mcp` directory and throwing `state.json` into the trash felt incredibly satisfying. Moving all logic, parsing, validation, and credentials decryption back to the centralized `mework-server` makes the daemon a simple, dumb poll-loop worker. But it also forced us to handle concurrency properly at the server level, which was painful to debug during initial schema testing.

## Technical Details

### Concurrency and Locking
In `internal/server/jobs/claim.go`, we serialize claims per runtime first to prevent multiple threads from claiming different jobs for the same runner simultaneously:
```go
// 1. Concurrency limit: pg_advisory_xact_lock to serialize claims per runtime
_, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(hashtext($1))", runtimeID)
```

Then we query and update the oldest queued job, skipping locked rows to avoid contention:
```go
// 3. Claim oldest queued job
err = tx.QueryRow(r.Context(), `
    UPDATE jobs
    SET status = 'claimed', claim_lease_until = NOW() + INTERVAL '30 seconds', attempts = attempts + 1
    WHERE id = (
        SELECT id FROM jobs
        WHERE runtime_id = $1 AND status = 'queued'
        ORDER BY created_at ASC
        LIMIT 1
        FOR UPDATE SKIP LOCKED
    )
    RETURNING id, ...
`, runtimeID).Scan(...)
```

### Webhook Trigger Grammar
The webhook parser searches for the `@mework` trigger and extracts parameters:
```go
// ParseTrigger parses the trigger grammar from a comment body:
// "@mework [profile-name] [workflow-name] [free instructions]"
func ParseTrigger(body string) (profile, workflow, instructions string, ok bool)
```

### E2E Test Suite Run
We verified the E2E pipeline by spinning up a mock server in `internal/integration/pipeline_test.go`:
```bash
make test-db && TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework_test?sslmode=disable" go test -v ./internal/integration
```
Output:
```
=== RUN   TestFullPipelineE2E
2026/06/16 11:36:59 [mbp14.local/EghqZSG4cr-000001] "POST http://127.0.0.1:51955/api/v1/runtimes HTTP/1.1" - 201 278B in 7.20ms
2026/06/16 11:36:59 [mbp14.local/EghqZSG4cr-000004] "POST http://127.0.0.1:51955/webhooks/mello HTTP/1.1" - 202 0B in 7.82ms
2026/06/16 11:36:59 [mbp14.local/EghqZSG4cr-000005] "POST http://127.0.0.1:51955/api/v1/jobs/claim HTTP/1.1" - 200 478B in 12.30ms
--- PASS: TestFullPipelineE2E (0.58s)
PASS
ok      mework/internal/integration     0.821s
```

## What We Tried

- **Client-Side MCP Write-back**: Initially proposed to keep the client-side MCP client running inside the daemon. We rejected this because it required the daemon to manage access tokens directly, increasing security exposure on runner nodes, and added needless dependency complexity (FastMCP/Stdio).
- **Polling for Webhook Events on Daemon**: Considered having the daemon poll Mello's API directly for comments. Rejected in favor of the server webhook parser enqueuing to a centralized job store, reducing API rate limit hits on Mello and providing a single source of truth for job execution audit logs.

## Root Cause Analysis

The initial coupling of daemon state (`state.json`) and direct write-back credentials to the runner nodes was a scaffolding shortcut that ignored production operations constraints. Runtimes must be treated as untrusted, disposable, and stateless execution environments. Any architecture requiring runtimes to hold high-privilege board personal access tokens (PATs) or self-manage execution history is fundamentally insecure and prone to concurrency deadlocks.

## Lessons Learned

1. **Keep Runtimes Stateless**: Runtimes should only pull, run, and report status. Persisting state locally on ephemeral agents creates split-brain and synchronization failures.
2. **Centralize Secret Decryption**: Connection tokens should only be decrypted on the server and used to hit integration endpoints from the server side. Ephemeral runtimes must never see credentials for other integration targets.
3. **Database Concurrency Control is Crucial**: When building poll-based job queues, standard `SELECT` queries can lead to race conditions. Always use advisory locking (`pg_advisory_xact_lock`) to serialize actions per actor, and `FOR UPDATE SKIP LOCKED` to lock rows during queue popping.

## Next Steps

- **Monitor Job Table Growth**: Set up an autovacuum/pruning process for the `jobs` table to prevent it from growing indefinitely.
- **Implement Heartbeat Timeout Alerts**: Ensure there is alerting if a runtime claims a job but fails to send a heartbeat within 60 seconds (lease timeout).
