## 1. Real `session list` (TDD)

- [x] 1.1 Write `cmd_session_test.go` (fail first): `httptest` stub for `GET
      /api/v1/sessions` returning sessions; assert the table render and `--json` output;
      missing PAT → guidance error.
- [x] 1.2 Replace the stub `sessionListCmd` body with a real `GET /api/v1/sessions` (PAT
      auth), rendering the existing table header + rows.

## 2. `session create` (TDD)

- [x] 2.1 Write a test (fail first): `create --agent X [--runner R] [--version V] [--json]`
      POSTs `/api/v1/sessions` with the right body and prints the returned session id.
- [x] 2.2 Implement `sessionCreateCmd`.

## 3. `session send` (TDD)

- [x] 3.1 Write a test (fail first): `send <id> <msg>` POSTs
      `/api/v1/sessions/{id}/messages` with `{role:"user", content}` and treats 202 as
      success.
- [x] 3.2 Implement `sessionSendCmd`.

## 4. `session attach` (TDD)

- [x] 4.1 Write a test (fail first): stub an SSE response with two `data:` `ChatEvent`
      frames and a terminal `done`; assert the command prints the content and exits on
      `done`; assert an idle timeout exits cleanly when no terminal arrives.
- [x] 4.2 Implement `sessionAttachCmd`: open `GET /api/v1/sessions/{id}/stream`, decode
      frames, print `token`/`message` content, stop on `done`/`error` or idle timeout.

## 5. `session close` + registration

- [x] 5.1 Implement `sessionCloseCmd` (`DELETE /api/v1/sessions/{id}`).
- [x] 5.2 Register `create`/`send`/`attach`/`close` under `sessionCmd` in `init()`.

## 6. Validation

- [x] 6.1 `make vet` and `make test ./libs/client/cli/...` green; new tests fail-first then
      pass.
- [ ] 6.2 `make test` (full) green.
- [ ] 6.3 Manual E2E (with `c0030`–`c0033` + a running server): enroll → `daemon start` →
      `session create` → `session attach` (terminal A) + `session send` (terminal B) →
      streamed reply → `session close`.
