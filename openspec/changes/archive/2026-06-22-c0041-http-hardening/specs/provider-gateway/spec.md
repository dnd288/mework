## ADDED Requirements

### Requirement: Server HTTP hardening

The server SHALL bound the size of inbound request bodies and SHALL bound the time allowed to
read request headers, to mitigate memory-exhaustion and slow-client (slowloris) denial of
service. These limits MUST NOT apply to the long-lived Server-Sent-Events response streams
(the runtime subscribe stream and the session event stream), which remain open for the
lifetime of the subscription.

#### Scenario: Oversized request body is rejected

- **WHEN** a client sends a request whose body exceeds the configured maximum
- **THEN** the server rejects it rather than buffering an unbounded body

#### Scenario: SSE streams are not severed by the limits

- **WHEN** a subscriber holds an SSE stream open beyond the header-read timeout
- **THEN** the stream continues to receive events (the body/header limits do not close it)

### Requirement: Liveness and readiness probes

The server SHALL expose distinct **liveness** and **readiness** probes. Liveness SHALL report
process health independently of the database, so a transient database outage does not flap
liveness. Readiness SHALL report whether the server can serve traffic (the database is
reachable). Probe responses that depend on the database MUST return a generic status body and
MUST NOT leak the underlying error to the caller (the error is logged server-side). A
backward-compatible health endpoint MAY be retained with readiness semantics.

#### Scenario: Liveness independent of the database

- **WHEN** the database is unreachable but the process is running
- **THEN** the liveness probe still returns success

#### Scenario: Readiness reflects database reachability without leaking errors

- **WHEN** the database is unreachable
- **THEN** the readiness probe returns a not-ready status with a generic body, and the
  underlying database error is logged rather than returned to the caller
