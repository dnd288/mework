## Context

The hub dispatches agents to runners and the runner runs them inside sandboxes.
Neither side of that pipeline has the controls it needs: the dispatch side has no
planner that decides **which** runner should take the work (and just picks one),
and the sandbox side has no path to deliver **secrets** to the agent without
exposing them to argv or logs. Together these are the platform's load-balancing
and least-privilege primitives.

The contracts are small focused ports in their own modules so they can be built
and tested in isolation. Selection lives server-side (it owns the runner index);
injection lives in the client/sandbox runtime (it owns the lifecycle of the
sandbox process and the files/env it exposes to the agent).

The behaviors are pinned by the e2e scenarios in `tests/e2e/22_selection_secrets_test.go`.

## Goals / Non-Goals

**Goals:**

- A `RunnerSelector` that load-balances dispatches across eligible online runners,
  honours session affinity, and surfaces the no-eligible-runner case as an error
  rather than silently dropping the dispatch.
- A `SecretInjector` that delivers only grant-scoped secrets into a provisioned
  sandbox, out-of-band (env / file), never via argv or logs.

**Non-Goals:**

- The dispatch / grant / run lifecycle itself (`c0003` / `c0004`) — these ports
  wrap it.
- A full secrets manager / vault; the `SecretInjector` is the integration point,
  not the store.
- Cross-region scheduling / geo-affinity (covered by the basic load-balance; no
  topology is assumed).
- Quotas, audit, notifications, artifacts — owned by the sibling changes
  `c0014a` and `c0014b`.

## Decisions

- **`RunnerSelector.Select(ctx, tenant, criteria)`** returns `(RunnerID, error)`.
  `criteria` carries the agent ref and an optional session id. With multiple
  eligible runners, the selector picks the runner with the fewest active
  dispatches (ties broken by id for determinism). With a session id and a bound
  runner that is still eligible, affinity wins; if the bound runner is gone, fall
  through to load-balance. The no-eligible-runner case is a typed error
  `ErrNoEligibleRunner`, never a nil with an empty id.

- **Eligibility.** A runner is eligible for a dispatch when it is `online`, has
  not exceeded its per-runner concurrency (currently one active dispatch from
  `c0004`), and the dispatch's grant matches the runner's enrolled grants
  (e.g. a runner enrolled with only `claude-code` cannot take a `codex` agent).

- **`SecretInjector.Inject(ctx, sandbox, grant, secrets)`** materializes each
  granted secret into a per-sandbox file with `0400` permissions, owned by a uid
  that lives only inside the sandbox, and exposes it via an environment variable
  whose name is grant-scoped. The agent process inherits the env and reads the
  file directly; the secret value never appears in argv or in any log line the
  runner streams to the hub.

- **Scope enforcement.** Every `Inject` call verifies each `SecretRef.Source` is
  contained in `grant.Sources`. A request to inject a source not in the grant is
  refused with a typed error and the sandbox is aborted (the run fails fast rather
  than running with partial secrets).

- **Per-dispatch scope.** The injected env / files are scoped to the sandbox that
  ran the dispatch and are removed on teardown. Two dispatches on the same runner
  in sequence do not see each other's secrets.

## Risks

- **Affinity staleness.** A session's bound runner can be drained or go offline
  between dispatches. The selector falls through to load-balance when the bound
  runner is no longer eligible.
- **Secret materialization cost.** A large number of secrets per dispatch could
  slow provisioning. The default is to materialize in parallel; the cap is
  configurable per runner.
- **Argv scrubbing.** Tests assert that the secret does not appear in argv or
  logs; this depends on the runner not echoing the env at log time. The runner's
  log formatter redacts values matching `<GRANT_NAME>_<SECRET_NAME>` patterns.
