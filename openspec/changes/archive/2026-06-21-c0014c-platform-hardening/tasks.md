# Tasks

## 1. Runner selection (`server/orchestrator`)

- [ ] 1.1 Define the `RunnerSelector` port (`Select(ctx, tenant, criteria) (RunnerID, error)`) and the `Criteria` shape (`AgentRef`, `SessionID?`).
- [ ] 1.2 Persist per-runner presence (online / draining / offline) from `c0005-agent-runner` heartbeats; maintain an in-memory eligible-runner index keyed by `(tenant, agentRef)`.
- [ ] 1.3 Implement load-balancing: pick the eligible runner with the fewest active dispatches (ties broken by id for determinism).
- [ ] 1.4 Implement session affinity: if `Criteria.SessionID` is set and the session is bound to a runner that is still eligible, return that runner; otherwise fall through to load-balance.
- [ ] 1.5 Surface no-eligible-runner as a typed error `ErrNoEligibleRunner`; the dispatch path queues the dispatch for retry rather than dropping it.
- [ ] 1.6 Cover SELECT-01..03: load-balance spreads across runners, session affinity sticks, no-eligible-runner surfaces and is queued.

## 2. Grant-scoped secret injection (`client/sandbox`)

- [ ] 2.1 Define the `SecretInjector` port (`Inject(ctx, sandbox, grant, secrets) error`) and the `SecretRef` shape (name, source).
- [ ] 2.2 For each `SecretRef`, verify its `source` is contained in the dispatch's grant (`grant.Sources.Contains(source)`); refuse otherwise.
- [ ] 2.3 Materialize the secret value into a per-sandbox file at a path only readable inside the sandbox (e.g. `/run/mework/secrets/<name>`); set permissions `0400` and root ownership so the host cannot read it.
- [ ] 2.4 Expose the secret to the agent via an environment variable whose name is the grant-scoped env name (`<GRANT_NAME>_<SECRET_NAME>`); the agent reads `getenv` and the value never appears in argv.
- [ ] 2.5 Capture argv and streamed logs for the run; assert via test that the secret value does not appear in either. (Test plan: SECRET-02.)
- [ ] 2.6 Cover SECRET-01..03 + the per-dispatch scope scenario: granted secret injected, secret absent from argv/logs, out-of-grant refused, per-dispatch scope enforced across the same runner.

## 3. Wire selection into the dispatch path

- [ ] 3.1 In `c0004-agent-catalog` dispatch handler, call `RunnerSelector.Select` after the quota check; on `ErrNoEligibleRunner` enqueue for retry.
- [ ] 3.2 Persist the bound `(sessionID, runnerID)` so the next dispatch in the session resolves through the affinity path.
- [ ] 3.3 In `c0006-sandbox-runtime` sandbox provisioning, call `SecretInjector.Inject` with the dispatch's grant before launching the agent process.

## 4. Validate

- [ ] 4.1 `go test ./tests/e2e/22_selection_secrets_test.go` is runnable (currently `t.Skip` on SELECT/SECRET); implement the `FakeRunnerSelector` / `FakeSecretInjector` and per-scenario assertions until all green.
- [ ] 4.2 `make vet` and `make test` stay green.
- [ ] 4.3 `openspec validate c0014c-platform-hardening --type change --strict` passes.
