# Proposal: Runner load-balancing and grant-scoped secret injection

## Why

When the hub dispatches an agent, it has to pick a **runner**: it should not dump
all work onto one machine (load balance), and a follow-up dispatch on a session
should go back to the runner that owns the session (affinity). Once the runner is
chosen, the agent inside its sandbox needs to reach secrets — but those secrets
are scoped to the dispatch's grant and the agent MUST NEVER receive them via argv
or in streamed logs. Neither control exists today.

## What

Introduce two small, focused ports and pin them with the e2e scenarios
`SELECT-01..03` and `SECRET-01..03`:

- **Runner selection (`server/orchestrator`)** — a `RunnerSelector` port (`Select`)
  that load-balances dispatches across eligible online runners and honours session
  affinity, surfacing the no-eligible-runner case as an error rather than silently
  dropping it.
- **Grant-scoped secret injection (`client/sandbox`)** — a `SecretInjector` port
  (`Inject`) that delivers only grant-scoped secrets into a provisioned sandbox,
  out-of-band (env or file), never via argv or logs.

## Impact

- **Depends on c0000-tenancy** (runner eligibility is per-tenant).
- **Depends on c0005-agent-runner** (runner presence and lifecycle).
- **Depends on c0006-sandbox-runtime** (the sandbox is the injection target).
- **Depends on c0004-agent-catalog** (the grant is the scope).
- Module homes: `server/orchestrator` (`RunnerSelector`),
  `client/sandbox` (`SecretInjector`).
- Behaviors are pinned by `tests/e2e/22_selection_secrets_test.go`.

## Capabilities

### New Capabilities

- `platform-selection-secrets`: runner selection with load-balancing and session
  affinity, and grant-scoped secret injection into a sandbox out-of-band (env/file),
  never argv or logs.

## Sibling

This is one of three splits of the original `c0013-platform-hardening`. The other
two are:

- `c0014a-quotas-audit` — per-tenant quotas/rate limits and the tenant-scoped audit
  log.
- `c0014b-notify-artifacts` — outbound notifications with bounded retry, and a
  run-scoped artifact store.
