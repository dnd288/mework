## Context

The server is sound on auth/sealing but missing standard HTTP-edge hardening. The one
constraint shaping every decision is **SSE**: `/jobs/subscribe` and `/sessions/{id}/stream`
hold long-lived response streams, so naive `WriteTimeout`/`Throttle`/request-`Timeout`
middleware would sever them.

## Goals / Non-Goals

**Goals:** bound request bodies; mitigate slowloris; split liveness from readiness without
leaking errors; reject weak keys — all SSE-safe.

**Non-Goals:** per-route rate limiting / global `Throttle` (would starve long-lived SSE
connections — deferred, needs SSE-aware accounting); a full metrics/tracing stack (that is
`c0042`); WAF-style request inspection.

## Decisions

- **`RequestSize` over a hand-rolled `MaxBytesReader`.** `chimiddleware.RequestSize(4<<20)`
  wraps `r.Body` in a `MaxBytesReader` globally. SSE requests carry a trivial body, so the
  cap is invisible to streaming; only oversized inbound bodies (e.g. webhooks) are rejected.
- **`ReadHeaderTimeout`/`IdleTimeout`, never `WriteTimeout`.** Header-read time is bounded
  (slowloris) without touching the response stream. `IdleTimeout` (120s) is comfortably
  above the SSE heartbeat interval so idle-looking-but-alive streams survive.
- **Three probes.** `/livez` = process up (no DB). `/readyz` = can serve (DB ping).
  `/healthz` is kept as a readiness alias for existing probes. All DB-dependent responses
  return a generic body and log the real error — orchestrators key on the status code, not
  the body, and the driver error is operator-only.
- **Min key length 16.** Enforced in `LoadConfig` alongside the presence check. 16 chars is
  a low, non-disruptive floor that still rejects the trivially-weak keys the assessment
  flagged; the value is a named constant for easy tightening.

## Risks / Trade-offs

- **[Existing deploys with <16-char keys fail to start]** → intended fail-fast; the error
  names the variable and the minimum. Documented in the deployment guide.
- **[4 MiB cap too low for some payload]** → it is a named constant; webhook bodies and API
  requests are far smaller. Raise if a provider needs it.
- **[No rate limiting yet]** → explicitly deferred; the body cap + header timeout remove the
  cheapest DoS vectors without risking SSE.

## Migration Plan

Additive and backward-compatible except the key-length floor (fail-fast). No schema or API
breakage; `/healthz` semantics preserved (now non-leaking).
