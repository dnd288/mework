# Test Results — 2026-06-22 10:11 UTC

## go vet ./...
```
PASS
```

## Go tests (all packages, serial)

| Package | Result | Time |
|---------|--------|------|
| `libs/server/session` | ok | 0.175s |
| `libs/client/runner` | ok | 4.300s |
| `libs/server/hub` | ok | 2.587s |
| `libs/server/auth` | ok | 1.833s |
| `libs/server/registry` | ok | 14.154s |
| `libs/server/webhook` | ok | 1.524s |
| `libs/server/orchestrator` | ok | 3.586s |
| `libs/server/platform/store` | ok | 1.326s |
| `libs/sandbox/runtime` | ok | 0.002s |
| `libs/shared/transport` | ok | 0.002s |

## Python E2E (examples/remote-claude/scripts/e2e.py)
```
PASS — full server → daemon → sandbox → chat flow verified
```
