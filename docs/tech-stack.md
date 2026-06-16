# Mework CLI Daemon and Server — Tech Stack

Status: approved-implemented
Date: 2026-06-16

## Decision summary
A Go CLI and agent-runtime daemon (`mework`) integrated with a central webhook-driven server (`mework-server`).
- The **mework-server** acts as the central orchestrator, receiving provider webhooks (e.g., Mello), parsing trigger keywords (e.g., `/run`), managing job states via a PostgreSQL database queue, and handling secure write-backs back to Mello.
- The **mework daemon** is a lightweight agent runner that polls the central server via standard HTTP (`POST /api/v1/jobs/claim`) using a registered `rt_token`, executes the local AI CLI (claude / codex / opencode) against the ticket context in an isolated workspace, and returns execution status and outputs via `/ack` endpoints.

## Core Stack

### Common & CLI
| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.23+ | Single static binary; excellent concurrency; cross-compilable via goreleaser. |
| CLI framework | `github.com/spf13/cobra` | Standard CLI framework; supports command groups (core, runtime, additional). |
| Config | JSON at `~/.mework/config.json` | Config store with `--profile` directory isolation. |
| Release | goreleaser + Makefile | Versioned archives, automated builds, self-update machinery. |
| Tests | stdlib `testing` | Unit tests for CLI and daemon loops. |

### Central Server (`mework-server`)
| Concern | Choice | Rationale |
|---|---|---|
| HTTP Router | `github.com/go-chi/chi/v5` | Lightweight, idiomatic Go router with middlewares. |
| Database | PostgreSQL | Sturdy, transactional storage for jobs, connections, runtimes, and profiles. |
| SQL Driver | `github.com/jackc/pgx/v5` | High-performance PostgreSQL driver and connection pool (`pgxpool`). |
| Database Migrations | `github.com/pressly/goose/v3` | Embedded SQL migrations for robust database schema management. |
| Secrets Encryption | AES-256 GCM | Encrypts provider access tokens in database using a 32-byte `MEWORK_SECRET_KEY`. |
| Concurrency Control | PostgreSQL Advisory Locks | Transaction-level locks for claim logic to guarantee exactly-once job execution. |

### Local Daemon
| Concern | Choice | Rationale |
|---|---|---|
| Server Client | `meworkclient` | Custom internal Go client SDK for server API communication. |
| Daemonize (Unix) | `syscall.SysProcAttr{Setsid:true}` | Detach daemon process from current shell session. |
| Daemonize (Windows) | `DETACHED_PROCESS` + `CREATE_BREAKAWAY_FROM_JOB` | Detach daemon process to survive console window closure. |
| AI Runner | `internal/agentrun` | PATH detection for local AI CLIs; spawns processes and feeds context securely via stdin. |

## Layout

```
cmd/mework/            # CLI commands (Cobra entry point + command groups)
cmd/mework-server/     # Server main entry point, config, and router initialization
internal/cli/          # CLI configuration, profiles, and flags precedence
internal/meworkclient/ # Server HTTP client SDK (job claim/ack, runtime registry, connection, profile CRUD)
internal/server/       # Webhook adapter, registry, jobs queue (claim/ack), profiles, and connections services
internal/store/        # PostgreSQL connection pool and embedded Goose migrations
internal/mello/        # Mello REST API client implementation
internal/agentrun/     # Local AI CLI launcher (stdin flow, stdout/stderr capture, timeout handler)
internal/daemon/       # Daemon polling loop, heartbeat ticker, and health/shutdown handlers
```

## Config Keys (config.json)

| Key | Purpose |
|---|---|
| `server_url` | Mework central server endpoint (default: `http://localhost:8080`). |
| `rt_token` | Secure runtime registration token for daemon polling and authentication. |
| `token` | Mello personal access token (used by the local CLI for direct workspace/board operations). |
| `base_url` | Mello API base url (default: `https://mello.mezon.vn/api/v1`). |
| `workspace_id` | Default workspace for direct CLI operations. |
| `daemon.trigger_keyword` | Trigger keyword (default: `/run`). |
| `daemon.done_column_id` | Optional board column to move tickets to upon completion. |

## Resolved decisions
- **No MCP client in Daemon**: The daemon is completely decoupled from MCP. Writing comments back is offloaded entirely to `mework-server`'s outbound worker via the provider REST API.
- **Provider Encryption**: All provider keys and tokens are securely encrypted on the server database using AES-256-GCM.
- **Advisory Locks**: Transaction-level advisory locks prevent race conditions and guarantee that no two runtimes can claim the same job.
