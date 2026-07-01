## ADDED Requirements

### Requirement: Outbound notifications

The system SHALL deliver **outbound notifications** to a per-tenant configured target
on platform events, at least `run.done` and `run.failed`. Delivery MUST be retried on
transient failure up to a bounded retry limit so a flapping target does not drop
notifications.

#### Scenario: Run completion fires a notification

- **WHEN** a run completes and its tenant has a configured notification target
- **THEN** a run-completion notification is delivered to that target carrying the run id

#### Scenario: Run failure notifies the configured channel

- **WHEN** a run is finalized as failed
- **THEN** a failure notification carrying the run id is delivered to the configured target

#### Scenario: Delivery retries on transient failure

- **WHEN** a notification target returns a transient failure on the first attempt
- **THEN** delivery is retried until it succeeds or a bounded retry limit is reached

### Requirement: Run artifact store

The system SHALL persist run outputs as **artifacts** keyed by their run, over the
`object-storage` `ObjectStore` port. Artifacts MUST be retrievable individually and
listable per run, and each artifact's integrity MUST be verified against a recorded
checksum on retrieval.

#### Scenario: Run output is stored

- **WHEN** a finished run produces output and it is stored as an artifact
- **THEN** the artifact is persisted under that run via the object store

#### Scenario: Artifacts are retrievable by run

- **WHEN** a client fetches a stored artifact for a run
- **THEN** the stored bytes are returned

#### Scenario: Artifacts are listed per run

- **WHEN** a run's artifacts are listed
- **THEN** all artifacts stored for that run are returned

#### Scenario: Artifact integrity is checksum-verified

- **WHEN** an artifact stored with a checksum is retrieved
- **THEN** the recorded checksum is verified and a mismatch is detected and rejected

#### Scenario: Agents never hold store credentials

- **WHEN** an agent inside a sandbox retrieves or lists artifacts
- **THEN** the agent uses a presigned URL rather than holding the store credentials
  (covers the ARTIFACT variant where the runner/agent must not see the backend secret)
