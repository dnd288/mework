## 1. Catalog persistence

- [x] 1.1 Add migration for `agents` (name) and `agent_versions` (immutable version, `form`, payload/reference, checksum)
- [x] 1.2 Model moving pointers (e.g. `latest`, named channels) resolvable to a concrete version
- [x] 1.3 Provide a migration mapping existing `profiles` to `definition`-form agents

## 2. Catalog API

- [x] 2.1 `POST /api/v1/agents/{name}/versions` — publish an immutable version (reject overwrite)
- [x] 2.2 `GET /api/v1/agents` and `GET /api/v1/agents/{name}` — list / resolve (including `@latest`)
- [x] 2.3 `GET /api/v1/agents/{name}/versions/{version}/pull` — authorized pull, returns artifact or reference + `form`

## 3. Permission / policy model

- [x] 3.1 Define an enumerable operation set and a grant representation (scoped, least-privilege)
- [x] 3.2 Integrity-protect grants (sign/seal via `server/platform/{token,secret}`) so scope cannot be widened downstream
- [x] 3.3 Authorize pull and dispatch against caller identity + grant

## 4. Dispatch

- [x] 4.1 `POST /api/v1/agents/{name}/dispatch` — resolve version, build grant, publish a dispatch message to the target topic (via `message-bus`)
- [x] 4.2 Dispatch message references the exact version and carries the grant

## 5. Auth integration

- [x] 5.1 Extend authentication so grants attach to runner/agent/session identities
- [x] 5.2 Enforce that operations outside the current dispatch's grant are denied even for authenticated callers

## 6. Validation

- [x] 6.1 Tests: publish/immutability, resolve `@latest`, authorized vs unauthorized pull, dispatch carries grant
- [x] 6.2 `openspec validate --change agent-catalog --strict`
- [x] 6.3 e2e pointer: flip `tests/e2e/09_agent_catalog_test.go` from Skip to Green for CAT-01..10, and `tests/e2e/02_auth_grants_test.go` AUTH-07/08 (runner credential + grant-scoped operations) and GRANT-01..03 (tampered/absent/per-run-scope grants).
