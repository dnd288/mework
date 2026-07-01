## ADDED Requirements

### Requirement: Attach a session workspace mounted read-write

The system SHALL attach a folder to a session and mount it **read-write** at the
workspace's mount path so the sandbox can work in it, binding the workspace to a
remote object-store prefix.

#### Scenario: Attach mounts the folder read-write

- **WHEN** a workspace is attached to a session with a spec mounting at a given path in read-write mode
- **THEN** the workspace is mounted read-write at that path for the sandbox

#### Scenario: The sandbox writes a real file into the workspace

- **WHEN** the agent writes a file into the attached read-write workspace
- **THEN** the file exists and is readable back in the workspace

### Requirement: Writes sync to remote under the session prefix

The system SHALL sync workspace writes to the remote object store as objects under
the session's prefix, supporting sync modes `continuous` (mirror writes as they
happen), `on_flush` (sync on explicit flush or detach), and `manual` (only on an
explicit sync). A forced sync MUST be available and its outcome MUST be observable.

#### Scenario: A written file is pushed to remote

- **WHEN** a file is written into a synced workspace and sync runs
- **THEN** the file appears as an object under the session's remote prefix

#### Scenario: Force sync and observe status

- **WHEN** sync is forced on a workspace with local changes and its status is queried
- **THEN** the status reports pushed, pulled, and failed counts and the last sync time

### Requirement: Detach flushes then unmounts

The system SHALL, on detach, perform a **final flush** of pending writes to remote
**before** unmounting the workspace, regardless of the configured sync mode.

#### Scenario: Detach performs a final flush before unmount

- **WHEN** a workspace with unsynced writes is detached
- **THEN** the pending writes are flushed to remote before the workspace is unmounted

### Requirement: Re-attach restores the workspace from remote

The system SHALL, when a new sandbox re-attaches the same session workspace,
restore prior files by pulling them back from the remote object store so work
resumes across runs.

#### Scenario: Re-attach rehydrates prior files

- **WHEN** a session's workspace was synced and the sandbox was destroyed, and a new sandbox re-attaches the same session workspace
- **THEN** the prior files are pulled back from remote so the run resumes where it left off

### Requirement: Workspaces are isolated per session

The system SHALL isolate each session's workspace so that one session's writes are
not visible to another session's workspace except through an explicitly published
folder in the shared root.

#### Scenario: One session cannot see another's workspace files

- **WHEN** two sessions each have their own workspace and one writes a file
- **THEN** the other session's workspace cannot read that file

### Requirement: Writes are confined to the grant's writable prefix

The agent-facing file view SHALL confine writes and removals to the grant's
writable prefix and MUST block path traversal, denying any write that resolves
outside the writable prefix — including via `..` segments.

#### Scenario: Traversal write outside the prefix is denied

- **WHEN** the agent attempts to write a path that traverses outside the writable prefix using `..` segments
- **THEN** the write is denied with the traversal blocked and the scope enforced

### Requirement: The agent never holds raw store credentials

The system SHALL perform workspace sync through the object store's presigned or
hub-proxied path so that the agent never receives the store's raw access keys.

#### Scenario: Sync uses presigned or hub-proxied access only

- **WHEN** a workspace syncs to the object store
- **THEN** sync uses presigned URLs or a hub-proxied path and the store's access keys stay server-side

### Requirement: Read across the read-only shared root

The system SHALL mount a **read-only** shared root that the agent may read across,
exposing all published folders, and MUST deny writes into the shared root.

#### Scenario: Agent reads across the shared root

- **WHEN** the agent lists and reads files under the shared root
- **THEN** it can read all published folders in the shared root

#### Scenario: Writing into the shared root is denied

- **WHEN** the agent attempts to write into another session's folder under the shared root
- **THEN** the write is denied because the shared root is not writable

### Requirement: Push publishes only the grant-allowed folder

The system SHALL publish only the grant-allowed folder into the shared namespace
and MUST deny a publish to any destination outside the allowed folder. A published
folder MUST be readable by other sessions through the shared root.

#### Scenario: Publishing the allowed folder succeeds

- **WHEN** the agent publishes its allowed folder to the shared namespace
- **THEN** that one allowed folder is published

#### Scenario: Publishing outside the allowed folder is denied

- **WHEN** the agent attempts to publish to a destination outside the grant-allowed folder
- **THEN** the publish is denied

#### Scenario: A published folder is readable by other sessions

- **WHEN** one session has published its folder and another session reads the shared root
- **THEN** the other session can read the published output

### Requirement: Grant scopes read broadly while confining write and push

The system SHALL treat `workspace.read`, `workspace.write`, and `workspace.push` as
distinct grant operations, where read is scoped broadly across the shared root while
write is confined to the session workspace and push is confined to the one allowed
folder (least privilege).

#### Scenario: Read is broad while write and push are confined

- **WHEN** a grant carries broad workspace read, session-confined workspace write, and a single allowed workspace push destination
- **THEN** read, write, and push are evaluated as separate scopes with read broad and write and push confined

### Requirement: Base code materialized before the run

The system SHALL materialize a workspace's base code into the workspace **before**
any hook or the agent runs, supporting a base source of `git` (clone a repo at a
revision), `archive` (unpack an archive), or `store` (copy a template prefix from
the object store).

#### Scenario: Base git repo is cloned during bootstrap

- **WHEN** a workspace whose base is a git repo at a revision bootstraps
- **THEN** the repo is cloned into the workspace before the agent runs

#### Scenario: Base from an archive or store template is materialized

- **WHEN** a workspace whose base is an object-store template prefix bootstraps
- **THEN** the template is materialized into the workspace, confirming the base source is pluggable across git, archive, and store

### Requirement: Lifecycle hooks bracket the run

The system SHALL run lifecycle hooks at the `init`, `pre_run`, `post_run`,
`pre_sync`, and `post_sync` stages, where `init` runs during bootstrap before the
agent, `pre_run`/`post_run` bracket the agent, and `pre_sync`/`post_sync` bracket
the remote push. A failing `init` or `pre_run` hook MUST abort the run (reported
failed) and tear down the sandbox.

#### Scenario: Init hook runs before the agent during bootstrap

- **WHEN** a workspace with an init hook bootstraps
- **THEN** the init hook runs before the agent and its output is captured

#### Scenario: Pre-run and post-run hooks bracket the agent

- **WHEN** the runner drives the run lifecycle for a workspace with pre-run and post-run hooks
- **THEN** the pre-run hook runs before the agent and the post-run hook runs after, both captured

#### Scenario: Post-sync hook runs after the remote push

- **WHEN** a sync completes for a workspace with a post-sync hook
- **THEN** the post-sync hook runs after the files reach remote

#### Scenario: A failing init hook aborts the run

- **WHEN** a workspace whose init hook exits non-zero bootstraps
- **THEN** the run is aborted and reported failed and the sandbox is torn down

### Requirement: Hooks run within the grant scope over stdin

The system SHALL run hooks within the workspace grant scope so a hook may write the
workspace but MUST NOT exceed the grant, and MUST deliver hook scripts to the
sandbox over **stdin, never argv**.

#### Scenario: A hook cannot exceed the grant

- **WHEN** an init hook attempts an action outside the grant, such as a network call when the grant omits network
- **THEN** the hook is confined to the run's least-privilege grant and cannot exceed it

#### Scenario: Hook script is delivered on stdin, not argv

- **WHEN** the sandbox runs a hook whose script contains attacker-influenced content
- **THEN** the script is delivered on stdin and never appears in argv
