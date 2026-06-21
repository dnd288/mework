## 1. Session contract in `shared`

- [ ] 1.1 Define `SessionStatus` (`active|idle|closed`) and the `SessionInfo` view (`id, tenant, runner, agent, status, owner, created`) in `shared`
- [ ] 1.2 Define the `SessionManager` interface (`Create`/`Get`/`List`/`Attach`/`Close`) in `shared`, with `Attach` returning the live bus `Session` endpoint

## 2. Session lifecycle in `server/session`

- [ ] 2.1 `Create(d Dispatch)`: turn a dispatch into a tracked session and persist `SessionInfo` with status `active` and the owning account
- [ ] 2.2 `Get(id)`: return the current `SessionInfo` for a session
- [ ] 2.3 `Close(id)`: terminate the session, destroy its sandbox, and mark it `closed` (terminal)

## 3. Attach & resume

- [ ] 3.1 `Attach(id)`: return the live wire endpoint (bus `Session`) for the live agent association
- [ ] 3.2 Re-attach after a dropped connection resumes the still-running agent without losing session state

## 4. Listing, status & isolation

- [ ] 4.1 `List(tenant)`: enumerate a tenant's sessions, each entry carrying its `status` and `owner`
- [ ] 4.2 Tenant-scope listings so no cross-tenant session is ever returned
- [ ] 4.3 Keep multiple sessions on one runner isolated (distinct ids, independent sandbox/control channel)

## 5. Ownership & idle reaping

- [ ] 5.1 Enforce ownership on `Attach`: deny any account that does not own the session
- [ ] 5.2 Idle reaper: when a session is idle past its timeout, transition it to `closed` and destroy its sandbox

## 6. Validate

- [ ] 6.1 Tests covering create→attach→close, list (status+owner), resume, multi-session isolation, idle reaping, ownership, tenant isolation
- [ ] 6.2 `openspec validate c0009-sessions --type change --strict`
- [ ] 6.3 e2e pointer: flip `tests/e2e/16_sessions_test.go` from Skip to Green for SESSION-01..07 (create→attach→close, list/status/owner, resume-after-reconnect, multi-session-per-runner, idle timeout, ownership, tenant isolation). Cross-references: `tests/e2e/10_runner_loop_test.go` LOOP-04/05 exercise the dispatch→run lifecycle that creates a session; `tests/e2e/17_chat_test.go` CHAT-04 (concurrent isolation) and `tests/e2e/14_concurrency_test.go` CONC-05 (concurrent sessions never cross-deliver) depend on per-session isolation.
