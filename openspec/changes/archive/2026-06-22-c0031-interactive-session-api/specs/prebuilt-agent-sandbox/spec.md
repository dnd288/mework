## ADDED Requirements

### Requirement: Server session lifecycle API and dispatch to a runner

The server SHALL expose an HTTP API to manage the lifecycle of an interactive session —
**create**, **get**, **list**, and **close** — authenticated as the human caller (PAT) and
**tenant-scoped to that caller** (never to a caller-supplied tenant argument). Creating a
session SHALL **dispatch an open-session message to the named runner** over the
control/dispatch bus, carrying the **session id**, the caller's **owner** and **tenant**,
and a permission **grant** authorizing the runner to pull the agent and spawn the sandbox
(`OpPullAgent | OpSpawn`). The dispatch message SHALL identify an open-session request by
carrying a non-empty session id (distinct from a one-shot agent run). The server SHALL
remain a gateway/registry per the runner-side-execution requirement: it stores session
metadata and dispatches, and it MUST NOT spawn a sandbox.

#### Scenario: Create a session over HTTP

- **WHEN** an authenticated caller issues `POST /api/v1/sessions` naming an agent and a
  runner
- **THEN** the server creates session metadata owned by the caller and scoped to the
  caller's tenant, returns the session descriptor, and dispatches an open-session message
  to that runner carrying the session id, owner, tenant, and a pull+spawn grant

#### Scenario: List and get are tenant-scoped

- **WHEN** an authenticated caller lists sessions or gets one by id
- **THEN** the server returns only sessions in the caller's own tenant, regardless of any
  tenant argument supplied

#### Scenario: Close a session over HTTP

- **WHEN** an authenticated caller issues `DELETE /api/v1/sessions/{id}` for a session it
  owns
- **THEN** the server closes the session metadata and stops relaying its control stream

#### Scenario: Open-session dispatch carries owner and tenant

- **WHEN** an open-session message is dispatched to a runner
- **THEN** it carries the owner and tenant so the runner can authorize subsequent turns
  against the session owner and tenant

### Requirement: Runner reports session results to the server

The server SHALL accept a **terminal result** for a session from the runner over an HTTP
endpoint authenticated by the runner's runtime credential. The endpoint SHALL accept a
status (e.g. done / failed / refused), an optional summary, and an optional error, and
acknowledge receipt. The server MAY surface a terminal result to session subscribers as a
terminal event.

#### Scenario: Runner posts a terminal result

- **WHEN** the runner posts a terminal result for a session to
  `POST /api/v1/runners/sessions/{id}/result` with its runtime credential
- **THEN** the server accepts and acknowledges it

#### Scenario: Result endpoint requires the runner credential

- **WHEN** a caller without a valid runtime credential posts to the result endpoint
- **THEN** the server rejects it as unauthorized
