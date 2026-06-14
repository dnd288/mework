# Database Schema Pivot to Generic Provider Architecture

**Date**: 2026-06-14 14:51
**Severity**: Medium
**Component**: Database Schema, Server Backend
**Status**: Resolved

## What Happened

We refactored and pivoted our database schema away from its tight coupling with Mello-specific types and fields (like `mello_user_id`, `mello_board_id`, `mello_ticket_id`, and `mello_comment_id`). The schema now implements a provider-agnostic architecture, facilitating integration with any issue tracker or project management provider (Jira, GitHub, Linear, etc.) via the same core table layout.

## The Brutal Truth

Building the schema around Mello-specific constructs early on was shortsighted and painful. It forced us to write code that assumed we were only ever connecting to Mello. As soon as we needed a general daemon architecture to route jobs for multiple external systems, we had to tear everything down and rewrite the entire migration script. It was a massive pain to go line-by-line through the schema and replace fields, knowing that any mistake would break the downstream daemon lifecycle logic. But avoiding this tech debt now saves us from a nightmare database migration later.

## Technical Details

We modified `internal/store/migrations/000001_init.sql` to support generic provider types. Below is a subset of the table updates:

```sql
CREATE TABLE provider_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_code VARCHAR(255) NOT NULL,
    webhook_secret TEXT,
    mcp_url TEXT,
    mcp_auth_enc TEXT,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, provider_code)
);
```

We also added performance-minded constraints and indexes to address query execution paths:
- `idx_jobs_account_id`: Explicit index on `jobs(account_id)` to prevent sequential table scans when executing `ON DELETE CASCADE` checks on the `accounts` parent table or filtering jobs by account.
- `idx_jobs_writeback`: A partial index `ON jobs (writeback_status) WHERE writeback_status = 'pending'` to accelerate polling jobs with pending writeback statuses.
- Replaced `VARCHAR(255)` with `TEXT` fields for `webhook_secret`, `mcp_url`, and `mcp_auth_enc` to handle long URLs and tokens without arbitrary length truncation.

Verification was completed by running:
```bash
TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/mework_test?sslmode=disable" go test -v ./internal/store/...
```
The integration test suite verified the migration script:
1. Ran `RollbackMigrations` to ensure a clean database start.
2. Ran `RunMigrations` to construct tables and assert their structures.
3. Inspected `information_schema` and `pg_constraint` to confirm the absence of obsolete Mello columns/tables and the presence of new constraints (e.g. UNIQUE keys on `watched_containers` and `provider_connections`).
4. Verified index filters and definitions using `pg_indexes`.
5. Executed a final teardown down migration to verify the database was left clean.

## What We Tried

- **Keep Mello Schema and Add Mapping Layer**: We considered keeping the schema as-is and performing client-side mapping for other providers (e.g. mapping `mello_board_id` to a Jira board ID). We rejected this because it makes database-level queries confusing, leaks third-party details into the core domain, and violates the clean separation of concerns.
- **Separate Tables per Provider**: We considered creating separate tables for each provider (e.g. `mello_connections`, `jira_connections`). We rejected this as it violates DRY, requires schema changes for every new provider, and makes the scheduler logic overly complex.

## Root Cause Analysis

The root cause was premature specialization. In the rush to build a working prototype, we named columns and tables after our first integration target (`mello_*`) instead of abstracting the domain models (accounts, connections, tasks, events, and containers).

## Lessons Learned

1. **Abstract Third-Party Namespaces**: Never name database schema columns after external brands or services unless they are genuinely isolated adapter tables.
2. **Postgres Indexing for Cascading Deletes**: Always create indexes on foreign keys that cascade delete, particularly on active transaction tables like `jobs`, to avoid full table scans.
3. **Use TEXT for Web Resources**: Do not assume URLs or secrets fit in `VARCHAR(255)`. Use `TEXT` for webhooks, MCP URLs, and credentials.

## Next Steps

1. **Update Daemon Code**: Align the daemon's internal event listener and writeback client to parse the new generic provider types.
2. **Implement Phase 02 API**: Implement account mapping and token lookup logic utilizing `provider_connections`.
