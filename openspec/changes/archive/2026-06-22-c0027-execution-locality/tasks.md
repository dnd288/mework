## 1. Spec

- [ ] 1.1 Add the **Runner-side execution; server is gateway and registry** requirement to the prebuilt-agent-sandbox delta spec (done in this change)

## 2. Enforcement

- [ ] 2.1 Add a `libs/tools/import-guard` rule forbidding `libs/server/**` from importing `mework/libs/sandbox/engine/*` or `mework/libs/sandbox/runtime`
- [ ] 2.2 Confirm the guard passes against current code (server already imports neither)

## 3. Docs

- [ ] 3.1 Fix the `examples/remote-claude/README.md` architecture diagram: put the daemon + sandbox in their own runner tier, with the server box holding only sessions / registry / bus
- [ ] 3.2 Align the README prose with the corrected diagram (server = gateway + registry; runner = daemon + sandbox)

## 4. Validation

- [ ] 4.1 `openspec validate c0027-execution-locality --strict` passes
- [ ] 4.2 `make vet` and `make test` green (import-guard rule included)
