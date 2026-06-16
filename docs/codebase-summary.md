# Codebase Summary

Centralized Go CLI and agent-runtime daemon (`mework`) integrated with a central server (`mework-server`) featuring a webhook-driven architecture for Mello (and other provider) integrations.

## Layout

```
cmd/mework/            Cobra commands (CLI entry point + command groups)
  main.go              root cmd, persistent flags, version, profile() helper
  help.go              command registration, config show/set
  client.go            REST client builder + server URL / workspace-id resolver
  output.go            table / --json rendering helpers
  cmd_auth.go          login, auth status/logout
  cmd_board.go         workspace + board commands
  cmd_ticket.go        ticket, comment, search commands
  cmd_daemon.go        daemon start/stop/status/restart/logs
  cmd_daemon_unix.go   Setsid detach (build tag !windows)
  cmd_daemon_windows.go DETACHED_PROCESS detach (build tag windows)
  cmd_provider.go      manages provider connection (provider connect)
  cmd_profile.go       manages server-side AI profiles (profile create/list/update/delete)
  cmd_runtime.go       registers runtimes and tokens (runtime register/list/revoke)
  cmd_version.go       version command

cmd/mework-server/     Mework central server entry point
  main.go              config load, migrations run, HTTP listen, graceful shutdown

internal/cli/          config + path + flag-precedence layer
  config.go            Config struct, Load/Save (JSON, 0600)
  paths.go             ~/.mework paths, profile isolation
  flags.go             FlagOrEnv, Resolve{BaseURL,WorkspaceID,Token}

internal/meworkclient/ Client SDK for daemon-to-server and CLI-to-server communications
  client.go            client definition, request runner
  connection.go        provider connection API calls
  job.go               job claim, ack, heartbeat API calls
  profile.go           AI prompt profile CRUD API calls
  registry.go          daemon runtime registry API calls

internal/server/       HTTP server, configuration, router, health handlers
  auth/                validates personal access tokens (PAT) for CLI operations
  connection/          service and handlers for provider connections CRUD
  jobs/                implements job queueing, claims, status/result updates, and claim reclamation
  middleware/          runtime authentication middleware checking rt_token
  profile/             service and handlers for server-side AI system profiles CRUD
  provider/            provider adapter interface definitions and registry
    mello/             adapter for interacting with Mello REST API and publishing updates
  registry/            handles daemon runtime registrations and secure tokens
  webhook/             endpoint for verifying signatures and enqueuing inbound jobs
  router.go            chi router setup with middlewares (request id, logger, recover)
  config.go            env config (DATABASE_URL, LISTEN_ADDR, SERVER_KEY, MEWORK_SECRET_KEY)
  health.go            GET /healthz database ping check

internal/store/        database connection pool and goose migrations
  db.go                pgxpool wrapper + stdlib sql database connector
  migrate.go           embedded goose migrations up/down runner
  migrations/          SQL migrations (accounts, provider_connections, account_identities, watched_containers, runtimes, profiles, jobs)

internal/mello/        REST client + entity models
  models.go            User, Workspace, Board, Column, …
  models_ticket.go     Ticket, TicketDetail, Comment, Checklist, …
  client.go            HTTP transport, useV1 base-switch, error parsing
  operations.go        read + write REST methods
  errors.go            APIError + exit-code mapping

internal/agentrun/     local AI CLI detection + execution
  detect.go            PATH lookup for claude/codex/opencode
  runner.go            spawn with stdin prompt, capture output, timeout

internal/daemon/       lightweight, reshaped poll consumer
  run.go               main loop: poll claim -> mark running -> heartbeat -> execute -> ack
  handler.go           agent invocation, stdout/stderr extraction, and cleanup
  lifecycle.go         pid read/write/liveness, log file, health port
  health.go            loopback /health + /shutdown server
```

## Data flow

1. **Provider Webhook**: The external provider pushes an event (e.g., a ticket comment containing `/run`) to the server's webhook endpoint (`POST /webhooks/{provider}`).
2. **Job Enqueue**: The server validates the webhook signature, maps the event to a registered connection, resolves the target account's profile, and enqueues a new job in the durable postgres database queue.
3. **Daemon Long-Poll**: The local daemon periodically polls the server for jobs using its runtime token via `POST /api/v1/jobs/claim`.
4. **Job Execution**: The server claims the oldest queued job using transaction-level advisory locks to guarantee exactly-once processing. The daemon updates the job status to `running` (`POST /api/v1/jobs/{id}/ack`), forks the local AI CLI (e.g., `claude-code`) inside an isolated workspace, and heartbeats the server (`POST /api/v1/jobs/{id}/heartbeat`) every 30 seconds to maintain the active lease.
5. **Ack & Write-Back**: The daemon acks the terminal status (`done` or `failed`) with execution results via `POST /api/v1/jobs/{id}/ack`. The server outbound worker picks up the result and writes it back to Mello.

## Key invariants

- Webhook signature verification prevents processing unauthorized inbound requests.
- Job claiming uses PostgreSQL advisory locks to guarantee exactly-once processing across runtimes.
- Heartbeats extend job leases; a background sweeper recovers jobs from crashed runtimes.
- Prompt execution is fed to the local AI CLI strictly via stdin (shell injection prevention).
- Config and secret files are written with 0600 permissions; profile directories with 0700.

## Test coverage

- **Unit tests**: Cover flag precedence, config round-trip + profile isolation, REST error-to-exit-code mapping, webhook parser logic, job state transitions, and daemon lifecycle.
- **Store tests**: goose SQL migrations up/down rollback, verifying schema tables (accounts, provider_connections, account_identities, watched_containers, runtimes, profiles, jobs) and their indexes.
- **Integration tests**: Verify server config loading, health check, webhook-to-job pipeline, claim-ack state machine, and provider write-backs.
