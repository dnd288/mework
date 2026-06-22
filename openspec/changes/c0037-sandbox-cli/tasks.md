## 1. `sandbox start` (TDD)

- [x] 1.1 Write `cmd_sandbox_test.go` (fail first): `sandbox start -w <tmp>` with a `mework.yml`
      present and a stubbed identity POSTs `/api/v1/sessions` with `{agent_name, version,
      runner, workspace=<abs tmp>}` and prints the returned session id (`--json` too). Missing
      `mework.yml` → error; missing identity (not enrolled) → guidance error.
- [x] 1.2 Implement `sandbox start -w <dir>` (default `.`): abspath; `LoadWorkspaceConfig`;
      `config.LoadIdentity` for the runner; POST create with workspace; print id; `--attach`
      streams `GET /sessions/{id}/stream`.

## 2. `sandbox list / stop / send`

- [x] 2.1 `sandbox list` → `GET /sessions` (reuse session list rendering).
- [x] 2.2 `sandbox stop <id>` → `DELETE /sessions/{id}`.
- [x] 2.3 `sandbox send <id> <msg>` → alias of `session send` (`POST /sessions/{id}/messages`).
- [x] 2.4 Register the `sandbox` group under `groupRuntime` in `help.go`.

## 3. Validation

- [x] 3.1 `make vet` + `make test ./libs/client/cli/...` green; new tests fail-first then pass.
- [ ] 3.2 `make test` (full) green.
- [ ] 3.3 Manual E2E (with c0035 + c0036, a running server + daemon, in a workspace dir):
      `mework sandbox start -w .` → prints id; `mework session attach <id>` streams;
      `mework sandbox send <id> "summarize the repo"` → reply streamed; `mework sandbox stop
      <id>`.
