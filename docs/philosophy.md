# Design Philosophy & Invariants

> Audience: developers and AI agents working in this repo. This is the "why" behind
> the architecture. The invariants below are non-negotiable — breaking one is a bug,
> not a style choice. They hold for both the current implementation and the planned
> agent hub.

## First principles

### 1. Source code and credentials stay local
The entire reason MeWork exists. The central server routes work and seals provider
credentials; it never receives your source tree. AI execution happens on a machine
you control, against your local checkout. A cloud AI coding service would require the
opposite — and that is precisely the trade we refuse to make.

### 2. Hybrid: a thin central brain, a local pair of hands
The server is provider-agnostic plumbing — routing, deduplication, durable delivery,
credential custody. The runner is where code is read and AI is run. Keeping these
separate means the server can be multi-tenant and centrally operated while execution
stays on the developer's device.

### 3. Least privilege, by default
Nothing is trusted more than it must be. The local side never holds provider
credentials. In the target architecture every dispatch carries a **scoped grant** and
the absence of a grant for an operation means that operation is **denied** — there is
no implicit "allow." Authentication establishes *who* a caller is; the grant
establishes *what this particular run may do*.

### 4. Defense in depth
Security does not rest on a single control. Untrusted input is constrained at every
layer it crosses: signature verification at the webhook edge, an actor allowlist
before enqueue, stdin-only prompt delivery at execution, and (target) a three-layer
permission model — **hub authorizes → runner enforces locally → sandbox contains.**

### 5. Durability over cleverness
A crash must never drop work or double-post a result. The job state machine is
transactional with row locks; the write-back outbox is a durable queue with
exactly-once delivery; idempotency is enforced by unique database constraints, not by
hopeful application logic.

### 6. Provider-agnostic to the core
A new provider is an adapter, never a migration. Entities are identified by
`(provider_code, external_*_id)`. Adding Jira or Linear means writing an adapter under
`libs/server/provider/<name>/` — the schema does not change.

### 7. KISS / YAGNI / DRY
Prefer the smallest design that satisfies the spec. Don't build for a future that
isn't specified. The message-broker backend defaults to Postgres `LISTEN/NOTIFY`
precisely so the target architecture adds **no new infrastructure** — NATS/Redis are
swappable later *if* a real need appears, behind an unchanged client contract.

## Security invariants (do not break)

| Invariant | Why | Where |
|-----------|-----|-------|
| **Prompts go to AI CLIs over stdin, never argv** | Ticket/agent content is attacker-controllable; keeping it off the command line prevents shell/command injection | `libs/sandbox/engine/local/runner.go` |
| **Job state machine is transactional with row locks; terminal states are immutable** | Prevents lost updates and re-running finished work. Allowed: `queued→claimed\|failed`, `claimed→running\|done\|failed\|queued`, `running→done\|failed\|queued`; same-status is a no-op | `libs/server/orchestrator/state.go` |
| **Webhook de-dup via `UNIQUE(provider_code, external_event_id)`** | A redelivered webhook must produce at most one job / one published message | `libs/server/platform/store/migrations/` + webhook handler |
| **One active job per runtime** (partial unique index) + `FOR UPDATE SKIP LOCKED` | Backstops concurrent claims; a runner runs one thing at a time | `libs/server/orchestrator/claim.go` |
| **Self-retrigger guard** | Never enqueue a job for a comment authored by the runner's own provider user — prevents infinite feedback loops | webhook handler |
| **Credentials sealed with AES-256-GCM at rest; unsealed only server-side at write-back** | The local side must never be able to read provider credentials | `libs/server/platform/secret/` |
| **Runtime tokens stored only as an HMAC-SHA256 lookup hash** | A database leak must not yield usable tokens; plaintext is shown exactly once at creation | `libs/server/platform/token/` |
| **File perms `0600` for credential/config files, `0700` for dirs** | Defense against other local users reading tokens/config | `libs/client/cli/`, `libs/client/runner/` |
| **(Target) grants are integrity-protected (signed/sealed) and least-privilege** | A runner must not be able to widen its own scope (grant-forgery defense) | `c0003-agent-catalog` |

## How these principles shape the redesign

The agent-hub redesign is not a rewrite of the philosophy — it is the philosophy made
sharper:

- **Least privilege** graduates from "the daemon holds no credentials" to "every run
  carries an explicit, signed, scoped grant."
- **Defense in depth** graduates from "stdin + isolated dir" to "hub authorizes →
  runner enforces → sandbox (container) contains."
- **Local-first** is unchanged and reinforced: the runner is install-once and
  remote-driven, but execution and source access never leave the device.

See [architecture.md](architecture.md) for how each principle maps to a component, and
[auth-and-secrets.md](auth-and-secrets.md) for the authentication and grant model.
