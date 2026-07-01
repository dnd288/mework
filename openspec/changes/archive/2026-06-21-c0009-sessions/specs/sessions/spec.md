## ADDED Requirements

### Requirement: Session lifecycle (create → attach → close)

A session SHALL be a first-class object representing **one live agent
association/run**, with an explicit lifecycle owned by the `SessionManager`.
`Create` from a dispatch MUST produce a tracked session; `Attach` MUST return the
live wire endpoint for that session; `Close` MUST terminate the session.

#### Scenario: Create, attach, then close a session

- **WHEN** a session is created from a dispatch for an agent on a runner, then a client attaches to it and closes it
- **THEN** create yields a tracked session, attach returns a live endpoint, and close terminates the session cleanly

### Requirement: List sessions with status and owner

A `SessionManager` SHALL expose a management listing of sessions for a tenant, and
each entry MUST report its `status` (one of `active`, `idle`, `closed`) and its
`owner`.

#### Scenario: Operator lists active sessions

- **WHEN** an operator lists the sessions for their tenant
- **THEN** each returned entry reports its status (active, idle, or closed) and the account that owns it

### Requirement: Resume an attached session after reconnect

Attaching to a session MUST return the existing live association rather than
starting a new one, so a client that re-attaches to the same session id after its
connection drops SHALL reconnect to the still-running agent without losing session
state.

#### Scenario: Re-attach after a dropped connection

- **WHEN** a client whose connection dropped re-attaches to the same session id
- **THEN** it reconnects to the still-running agent and the session state is preserved

### Requirement: Multiple sessions per runner are isolated

A single runner SHALL host multiple sessions concurrently, and each session MUST
have a distinct identity with its own sandbox and control channel so the sessions
do not interfere with one another.

#### Scenario: Two sessions coexist on one runner

- **WHEN** two independent sessions are created on the same runner
- **THEN** they have distinct ids and isolated sandbox/control channels, and neither interferes with the other

### Requirement: Idle sessions are reaped

A session with no activity past its idle timeout MUST be reaped: it SHALL
transition to `closed` and its sandbox SHALL be destroyed, so idle sessions never
leak sandboxes.

#### Scenario: An idle session times out

- **WHEN** a session has no activity until its idle timeout elapses
- **THEN** the session transitions to closed and its sandbox is destroyed

### Requirement: Ownership enforced on attach

A session SHALL be attachable only by the account that owns it; an attach attempt
by any other account MUST be denied.

#### Scenario: A non-owner cannot attach

- **WHEN** an account that does not own a session attempts to attach to it
- **THEN** the attach is denied

### Requirement: Tenant isolation of listings

Session listings SHALL be scoped to a single tenant; a listing for one tenant MUST
NOT return sessions belonging to any other tenant.

#### Scenario: Cross-tenant sessions are not visible

- **WHEN** an operator lists sessions for their tenant while sessions exist in another tenant
- **THEN** only the requesting tenant's sessions are returned and the other tenant's sessions are never visible
