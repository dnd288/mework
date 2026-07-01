## ADDED Requirements

### Requirement: Workspace-bound server-routed session

A server-routed interactive session SHALL support being **bound to a local workspace
directory**. The session-create request MAY carry a workspace path; when present, the server
SHALL forward that path to the runner on the open-session dispatch, and the runner SHALL
resolve the agent definition from that workspace's `mework.yml` and bind the long-lived
sandbox to the directory. The server SHALL remain a relay per the runner-side-execution boundary: it
forwards the workspace path and never reads the workspace itself. When no workspace path is
provided, the session resolves its definition from the server catalog as before.

#### Scenario: Create a workspace-bound session

- **WHEN** a caller creates a session with a workspace path
- **THEN** the server dispatches an open-session message carrying that path to the runner,
  and the runner opens the sandbox bound to that directory using the workspace's `mework.yml`

#### Scenario: No workspace falls back to the catalog

- **WHEN** a caller creates a session without a workspace path
- **THEN** the runner resolves the definition from the server catalog, unchanged

#### Scenario: Server does not read the workspace

- **WHEN** a workspace-bound session is created
- **THEN** the server only forwards the workspace path; the workspace files are read solely
  on the runner
