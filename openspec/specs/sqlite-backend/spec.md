# ADDED sqlite-backend

## ADDED Requirements

### Requirement: SQLite persists the job queue and platform tables

The server SHALL persist its job queue, runtimes, profiles, agents, sessions,
audit log, and runner identity in SQLite when `DATABASE_URL` is set to a
SQLite DSN (`sqlite://<path>`, `:memory:`, or `file:<path>?<pragma>=…`).

#### Scenario: SQLite driver applies migrations on startup

- **WHEN** the server is started with `DATABASE_URL=sqlite:///tmp/mework.db` and no schema exists
- **THEN** all migrations in `libs/server/platform/store/sqlite/migrations/` apply, the tables are created, and the server reaches `/readyz`

#### Scenario: SQLite preserves data after server restart

- **WHEN** the server writes a job and is then killed and restarted with the same `DATABASE_URL`
- **THEN** the job is still present after restart

### Requirement: Server fails closed on unsupported DATABASE_URL schemes

The server SHALL select its storage driver via the existing
`store.NewStore(ctx, cfg.DatabaseURL)` factory in
`libs/server/platform/store/`, which implements the `Store` interface. An
unsupported `DATABASE_URL` scheme SHALL cause `store.NewStore` to return a
non-nil error so the server exits non-zero at startup without silently falling
back to a default.

#### Scenario: Unsupported DATABASE_URL fails closed

- **WHEN** the server is started with `DATABASE_URL=redis://localhost:6379`
- **THEN** startup exits with a non-zero status and logs the unsupported scheme
- **AND** no data is written anywhere

### Requirement: SQLite driver uses pure-Go (no cgo)

The SQLite driver MUST use `modernc.org/sqlite` (pure Go) so no cgo toolchain
is required at build or install time.

#### Scenario: Server cross-compiles without gcc

- **WHEN** `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./apps/mework-server` is run
- **THEN** the binary builds without needing gcc installed

### Requirement: SQLite driver enables WAL, busy_timeout, foreign_keys on every connection

The SQLite driver SHALL enable `journal_mode=WAL`, `busy_timeout=5s`, and
`foreign_keys=on` on every connection it opens.

#### Scenario: Pragmas are set on a fresh connection

- **WHEN** the driver opens a new SQLite connection
- **THEN** querying `PRAGMA journal_mode` returns `wal`, `PRAGMA busy_timeout` returns 5000, and `PRAGMA foreign_keys` returns 1

### Requirement: SQLite concurrent job claim

The SQLite driver MUST allow multiple concurrent claimers on the `jobs` table
such that **exactly one** succeeds in claiming any given job. The driver SHALL
implement this using `BEGIN IMMEDIATE` followed by `UPDATE jobs SET
status='claimed', runner_id=?, claimed_at=? WHERE id IN (SELECT id FROM jobs
WHERE status='queued' AND provider_code=? ORDER BY created_at ASC LIMIT 1)
RETURNING *`. The equivalent semantics to Postgres `FOR UPDATE SKIP LOCKED`
are achieved by SQLite's writer-serialization and the `LIMIT 1` clause; if
the writer contends, the second claimer SHALL block until the first commits
or rolls back, then either re-evaluate or simply proceed (because the row is
no longer `status='queued'`).

#### Scenario: Two claimers, one job, one wins

- **GIVEN** a single queued job
- **WHEN** two goroutines attempt to claim it concurrently
- **THEN** exactly one claimer receives the job with `status='claimed'` and the other receives `nil, ErrNoJob`

#### Scenario: Five claimers, three jobs, all three claimed exactly once

- **GIVEN** three queued jobs
- **WHEN** five concurrent goroutines attempt to claim
- **THEN** exactly three claimers receive a job; the other two receive `ErrNoJob`; no job is claimed twice

### Requirement: SQLite no requirement for external services

The SQLite driver MUST NOT depend on any external daemon (no Postgres, no Redis,
no message bus). A single SQLite file is the entire database state.

#### Scenario: Server runs with only SQLite and no other services

- **WHEN** the server is started with `DATABASE_URL=sqlite:///tmp/mework.db` and **no other env vars set** beyond `SERVER_KEY` and `MEWORK_SECRET_KEY`
- **THEN** the server reaches `/readyz` and accepts traffic without crashing
