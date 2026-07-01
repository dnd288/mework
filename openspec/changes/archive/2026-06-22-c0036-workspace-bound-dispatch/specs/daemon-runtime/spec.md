## ADDED Requirements

### Requirement: Workspace-bound open-session dispatch

When an open-session dispatch carries a **workspace path**, the daemon SHALL resolve the
agent definition from that workspace's `mework.yml` (a local file resolver) instead of the
server catalog, and SHALL bind the long-lived sandbox to that directory so the agent runs
with the workspace as its working directory. The path is interpreted on the daemon host. When
the dispatch carries no workspace path, the daemon SHALL resolve the definition from the
server catalog as before. All other open-session behavior (one sandbox per session, serial
turns from the input topic, per-turn events to the server) is unchanged.

#### Scenario: Dispatch with a workspace binds the directory

- **WHEN** the daemon receives an open-session dispatch carrying a workspace path
- **THEN** it resolves the definition from that workspace's `mework.yml` and starts the
  sandbox bound to that directory

#### Scenario: Dispatch without a workspace uses the catalog

- **WHEN** the daemon receives an open-session dispatch with no workspace path
- **THEN** it resolves the definition from the server catalog, unchanged

#### Scenario: Missing workspace definition fails the session

- **WHEN** the dispatch's workspace path has no readable `mework.yml` on the daemon host
- **THEN** the daemon reports the session failed rather than starting an unbound sandbox
