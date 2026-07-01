# Offline Agent

## REMOVED Requirements

### Requirement: Offline agent can host a Mezon bot listener

**Reason**: The Mezon integration has been refactored into a standalone worker that requires a hub server. Offline mode (zero-infrastructure, no Postgres, no hub) cannot support the Mezon worker. The offline daemon's purpose is local-only operation via its Unix socket.

**Migration**: Users who want Mezon locally should run `mework server start` and the `mework-mezon-worker` as separate processes. Remove the `mezon.app_id` / `mezon.api_key` / `mezon.base_url` fields from `mework.yml` — they are no longer recognized by the offline daemon.

### Requirement: Mezon messages in offline mode use policy enforcement

**Reason**: Removed alongside the Mezon bot listener from offline mode. Policy enforcement remains for Unix socket messages (`"channel": "local"`).

**Migration**: Mezon messages are no longer processed by the offline daemon. Run the worker standalone.

### Requirement: Offline Mezon bot sends replies to channel

**Reason**: Removed — the worker handles replies independently via its outbound loop.

### Requirement: Offline mode credentials from mework.yml

**Reason**: Removed — `mezon` fields in `mework.yml` are no longer recognized. The worker reads credentials from environment variables instead.

### Requirement: Offline Mezon bot does not depend on Postgres or hub

**Reason**: Removed — the worker requires a hub server. Offline mode is local-only.
