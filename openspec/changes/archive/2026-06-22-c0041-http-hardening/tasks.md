## 1. Key strength (TDD)

- [x] 1.1 `config_test.go`: `LoadConfig` rejects `SERVER_KEY`/`MEWORK_SECRET_KEY` shorter than
      the minimum and accepts a valid-length key.
- [x] 1.2 `config.go`: add a `minKeyLen` (16) check after the presence checks.

## 2. Liveness/readiness split (TDD)

- [x] 2.1 `health_test.go`: `LivenessHandler` is 200 regardless of DB; `ReadinessHandler(nil)`
      is 503 with a generic `not ready` body (no leaked error).
- [x] 2.2 `health.go`: add `LivenessHandler` + `ReadinessHandler`; `HealthHandler` delegates
      to readiness; log (not return) the DB error.
- [x] 2.3 `router.go`: mount `/livez` and `/readyz` (keep `/healthz`).

## 3. Request-body cap + slowloris timeout

- [x] 3.1 `router.go`: add `chimiddleware.RequestSize(maxRequestBytes)` (4 MiB).
- [x] 3.2 `apps/mework-server/main.go` + `apps/mework/main.go`: set `ReadHeaderTimeout` +
      `IdleTimeout` on the `http.Server` (no `WriteTimeout` — SSE-safe).

## 4. Validation

- [x] 4.1 `make vet` + `go test ./hub/...` green (new config/health tests pass).
- [ ] 4.2 `make build` + `make test` (no DB) green.
- [ ] 4.3 `openspec validate c0041-http-hardening --strict` passes.
