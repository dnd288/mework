---
name: security-and-hardening
description: Hardens code against vulnerabilities (MeKnow-adapted, polyglot). Use when handling untrusted documents or webhook payloads, working with tenant isolation or server-side ACL in retrieve_kb, encrypting/decrypting provider credentials, the service JWT between backend and workers, or any answer-producing path. Use when building any feature that accepts untrusted data, manages tokens, or crosses a tenant boundary. Maps the OWASP Top 10 onto this repo's concrete controls (multi-tenant isolation + ACL-cohort cache keys, server-side ACL in retrieve_kb, Fernet credential encryption, HS256 service JWT, HMAC-signed inbound webhooks, citations-mandatory refusal, the compression invariant against KB prompt-injection).
tags: [security]
---

# Security and Hardening

## Overview

Security-first development practices for the Mezon Mentor Bot ("MeKnow")
multi-tenant RAG platform — the `backend`, the workers (`worker-ingest`,
`worker-retrieve`, `worker-task`, `worker-webhook`, …), and the shared `packages/`
(`rag-core`, `agent-core`, `arag-core`, `ingest-core`, `llm-transport`). Treat
every external input as hostile, every secret as sacred, every authorization check
as mandatory, and **every other tenant's data as off-limits**. Security isn't a
phase — it's a constraint on every line that touches a query, a cache key, KB
content, provider credentials, or the LLM/retrieval path.

Each project encodes its own concrete security controls as **invariants**. In an
mzspec-installed repo these live in `mzspec.config.json` under the `invariants`
array — read them first and map any new work onto them rather than inventing
parallel mechanisms. The table below is the **MeKnow reference set** (the example
this skill was extracted from); replace it with your project's `invariants` when
adapting the skill, and keep the methodology (OWASP-mapped, invariant-driven) intact.

## The MeKnow Security Invariants (reference example — do not break these in MeKnow)

| Invariant | What it defends | Where |
|---|---|---|
| **Multi-tenant by default** — `tenant_id` on every table, query, cache key, and log line; cross-tenant joins are bugs; cache keys include an ACL-cohort hash | Cross-tenant data leakage (the #1 risk) | every query path; cache layer |
| **ACL enforced server-side inside `retrieve_kb`** — caller identity inherited from the request, never trusted from the model | Broken access control; privilege confusion via the model | `retrieve_kb` in `rag-core`/`worker-retrieve` |
| **Provider credentials Fernet-encrypted**, decrypted only server-side | Secret exposure; blast radius if a worker host is compromised | `ingest_core.crypto` |
| **Service JWT (HS256) between backend and workers** | Spoofed inter-service calls | backend ↔ worker auth |
| **Inbound webhooks HMAC-signed and verified** before the payload is trusted | Spoofed events, forged ingests/triggers | `worker-webhook` |
| **Citations mandatory** — answer paths refuse rather than emit ungrounded claims (`filter` stage contains `refuse_if: no_citations`) | Ungrounded / fabricated output | `filter` stage; `agent-core` |
| **`temperature == 0`** on every `synthesize`, `cite`, `filter` stage | Non-determinism in grounded paths | policy stages |
| **Compression invariant** — raw KB chunks never leave a sub-agent; only `memo_schema_ref`-shaped objects cross stage boundaries | KB-content prompt-injection leaking across boundaries | sub-agent boundaries |
| **Versions are append-only** — `BotVersion`, `KBVersion`, golden sets, traces are new children, never in-place mutation | Tampering with the behavior/audit record | data model |
| **OpenAPI is the API contract** — portal types are generated, never hand-written | Drift between enforced contract and client | backend OpenAPI gen |

When you touch a subsystem, the invariant in that row is the control you must
preserve. The rest of this skill maps OWASP onto them.

## When to Use

- Building anything that accepts a request, webhook payload, or KB document content
- Working on the server-side ACL inside `retrieve_kb`, or any retrieval path
- Encrypting/decrypting or storing provider credentials (`ingest_core.crypto`)
- Issuing or verifying the service JWT between backend and workers
- Verifying inbound webhook HMAC signatures (`worker-webhook`)
- Ingesting documents (the untrusted KB → prompt-injection surface)
- Touching a query, cache key, or log line (the `tenant_id` surface)
- Handling any PII or credential data

## Process: Threat Model First

Controls bolted on without a threat model are guesses. Before hardening, spend
five minutes thinking like an attacker.

1. **Map the trust boundaries.** Where does untrusted data cross into the system?
   In MeKnow: **inbound webhook payloads**, **end-user questions** (which become the
   model prompt), **ingested KB documents** (which become retrieval context and can
   carry prompt-injection), provider API responses, and the **LLM's own output**
   (which is shown to users with citations). Every one is attack surface — and the
   **tenant boundary** sits across all of them.
2. **Name the assets.** What's worth stealing or breaking? **Another tenant's KB
   data and answers** (the crown jewels), provider credentials (Fernet-encrypted),
   the service-JWT signing secret, the Fernet key, the integrity of grounded answers
   (citations), and the append-only version record.
3. **Run STRIDE over each boundary** — a quick lens, not a ceremony:

| Threat | Ask | MeKnow mitigation |
|---|---|---|
| **S**poofing | Can someone forge a webhook or impersonate a worker? | Inbound webhook HMAC verify; service JWT (HS256) on backend↔worker calls |
| **T**ampering | Can data be altered in transit/at rest? | TLS; parameterized queries; append-only versions; Fernet AEAD on credentials |
| **R**epudiation | Can an action be denied later? | Append-only `BotVersion`/`KBVersion`/traces are an audit trail; tenant_id on every log line |
| **I**nformation disclosure | Can secrets or another tenant's data leak? | `tenant_id` + ACL-cohort cache keys; server-side ACL in `retrieve_kb`; Fernet at rest; compression invariant; never log secrets |
| **D**enial of service | Can it be overwhelmed? | p95 latency gate; bounded retrieval; request/document size caps; HTTP timeouts |
| **E**levation of privilege | Can a caller gain rights it shouldn't? | Server-side ACL with inherited caller identity; service-JWT scope; tenant scoping on every query |

4. **Write abuse cases next to use cases.** For each feature, ask "how would I
   misuse this?" — especially "can I read another tenant's data?" and "can a poisoned
   KB document make the model exfiltrate context?" — then make that your first test
   (`pytest` for Python; the gate scripts `tenant-isolation-test.sh` and
   `retrieve-kb-acl-test.sh` for the cross-cutting guarantees).

If you can't name the trust boundaries for a feature, you're not ready to secure
it. This is OWASP **A04: Insecure Design** — most breaches begin in design.

## The Three-Tier Boundary System

### Always Do (No Exceptions)

- **Scope every query, cache key, and log line by `tenant_id`.** Cross-tenant joins are bugs.
- **Enforce ACL server-side inside `retrieve_kb`** using the caller identity inherited from the request — never read it from model output or tool arguments.
- **Include the ACL-cohort hash in cache keys** so two callers with different KB permissions never share a cached answer.
- **Parameterize all queries** — never concatenate input into SQL.
- **Verify inbound webhook HMAC signatures** before acting on a payload.
- **Verify the service JWT** on backend↔worker calls; sign with the HS256 secret only server-side.
- **Encrypt provider credentials with Fernet** at rest (`ingest_core.crypto`); decrypt only server-side, only when needed.
- **Keep `refuse_if: no_citations`** in the `filter` stage; keep `temperature == 0` on synthesize/cite/filter.
- **Keep the compression invariant** — raw KB chunks stay inside the sub-agent; only `memo_schema_ref`-shaped objects cross boundaries.
- **Use TLS** for all provider and inter-service communication.
- **Fail fast** when a required secret (Fernet key, service-JWT secret, DB URL) is missing.
- **Run the dependency audit** (`uv pip`/`pip-audit`, `pnpm audit`, `govulncheck` for Go modules) before a release; keep lockfiles committed.

### Ask First (Requires Human Approval)

- Adding or changing authentication/authorization logic (service JWT or the `retrieve_kb` ACL)
- Changing the credential-encryption scheme (Fernet) or key handling
- Adding a new provider/connector or changing webhook signature verification
- Loosening tenant scoping, an ACL check, or CORS
- Changing cache-key composition (especially dropping `tenant_id` or the ACL-cohort hash)
- Relaxing `refuse_if: no_citations`, raising `temperature` on a grounded stage, or weakening the compression invariant
- Changing what gets logged near tokens, keys, credentials, or KB content

### Never Do

- **Never run a query, build a cache key, or write a log line without `tenant_id`.**
- **Never trust the caller identity (or ACL decision) from model output or tool args** — enforce it server-side.
- **Never let raw KB chunks cross a sub-agent boundary** (compression invariant).
- **Never commit secrets** — the Fernet key, service-JWT secret, provider credentials.
- **Never log sensitive data** — keys, decrypted credentials, service JWTs, or another tenant's KB content.
- **Never emit an answer with no citations** — refuse instead.
- **Never trust a webhook payload before its HMAC signature is verified.**
- **Never expose stack traces or internal errors** to API responses.
- **Never substitute a default for a missing required secret** — fail fast instead.

## OWASP Top 10, Mapped onto MeKnow

These are prevention patterns expressed in this repo's terms.

### A01 Broken Access Control — tenancy + server-side ACL (the #1 MeKnow rule)

The headline access-control surface here is **cross-tenant leakage** and the
**`retrieve_kb` ACL**. Every query and cache key must be tenant-scoped, and ACL must
be decided server-side from the inherited caller — never from anything the model
produced.

```python
# BAD: ACL decided from model/tool input; cache key not tenant/ACL scoped
chunks = retrieve_kb(query=q, allowed_acls=tool_args["acls"])  # model-controlled
cached = cache.get(query_hash(q))                              # leaks across tenants

# GOOD: caller inherited from the request; query + cache scoped by tenant + ACL cohort
chunks = retrieve_kb(tenant_id=ctx.tenant_id, caller=ctx.caller, query=q)
cached = cache.get(cache_key(ctx.tenant_id, q, acl_cohort_hash(ctx.caller)))
```

A cross-tenant join is a bug, full stop. These guarantees are gate-tested by
`benchmarks/gates/tenant-isolation-test.sh` and `retrieve-kb-acl-test.sh`.

### A03 Injection — SQL and KB-content prompt injection

Always parameterize queries (and keep the `tenant_id` predicate):

```python
# BAD: string-concatenated SQL
cur.execute(f"SELECT * FROM docs WHERE tenant_id = '{tenant_id}'")

# GOOD: parameterized
cur.execute("SELECT * FROM docs WHERE tenant_id = %s", (tenant_id,))
```

The subtler injection surface is **prompt injection from KB content** — see the
OWASP LLM section below. The compression invariant (raw chunks never leave a
sub-agent) is the structural control that contains it.

### A07 Identification & Authentication Failures — service JWT

The backend↔worker boundary authenticates with an HS256 service JWT. Verify the
signature and claims (issuer, audience, expiry) on every inter-service call; sign
only server-side with the secret.

```python
# Verify on the worker side: signature + claims, constant-time at the library layer.
claims = jwt.decode(token, service_secret, algorithms=["HS256"],
                    audience="worker-retrieve", issuer="backend")
# reject on any failure; never accept "alg": "none"
```

Never log the token. Pin `algorithms=["HS256"]` explicitly so an attacker can't
downgrade to `none`.

### A02 Cryptographic Failures — credential encryption (Fernet)

Provider credentials are encrypted with Fernet (`ingest_core.crypto`) and decrypted
only server-side, only when a worker needs them to call the provider.

```python
from cryptography.fernet import Fernet
# Encrypt at storage time (server-side). Fernet is AEAD: tampering is detected on decrypt.
token = Fernet(fernet_key).encrypt(secret_bytes)  # fernet_key from env, fail-fast if missing
# ... store `token` ...
secret_bytes = Fernet(fernet_key).decrypt(token)  # only where the provider call happens
```

The key comes from the environment and the service fails fast if it's missing.
Rotate by re-encrypting under a new key; never log the key or the plaintext.

### A04 Insecure Design — the invariants are the design

Tenant scoping, server-side ACL, the citations-mandatory refuse, `temperature == 0`
on grounded stages, append-only versions, and the compression invariant are
design-level controls. A "clever" shortcut that bypasses one of them is an insecure
design even if each line of code is fine. Preserve them; if a feature seems to
require breaking one, that's an Ask-First.

### A05 Security Misconfiguration

- Required secrets (Fernet key, service-JWT secret, DB URL) **fail fast** — don't add silent defaults.
- Don't leak internals: API error responses carry generic messages, not stack traces or DB errors.
- Keep `temperature == 0` and `refuse_if: no_citations` set in policy — a misconfigured stage that drops them is a security regression.
- Pin JWT `algorithms` and embedding/model versions; don't accept attacker-chosen ones.

### A08 Software & Data Integrity — webhook signatures + supply chain

```python
# Verify the inbound HMAC signature BEFORE parsing or acting on the payload.
expected = hmac.new(webhook_secret, raw_body, hashlib.sha256).hexdigest()
if not hmac.compare_digest(expected, sig_header):   # constant-time
    raise Unauthorized("invalid signature")
# only now: parse, then enqueue/ingest
```

For supply chain: commit lockfiles (`uv.lock`, `pnpm-lock.yaml`, `go.sum`), install
reproducibly in CI, run the per-toolchain audit (`pip-audit`/`govulncheck`/`pnpm
audit`), and review every new dependency (maintenance, license, footprint) — see the
`code-review-and-quality` sibling skill's dependency discipline section.

### A09 Security Logging & Monitoring Failures

Log security-relevant events (auth failure, signature-verify failure, ACL denial,
refusal) with **`tenant_id` + request context** — but **never** log keys, decrypted
credentials, service JWTs, or KB content. The append-only `BotVersion`/`KBVersion`/
trace records are your audit trail.

### A10 SSRF — provider/connector base URLs and ingest fetches

The ingest path and connectors fetch URLs (KB sources, provider APIs). If any
tenant-supplied config influences the host, an attacker can aim it at internal
services (cloud metadata `169.254.169.254`, `localhost`, private ranges).

```python
import ipaddress, socket
from urllib.parse import urlparse

def assert_safe_url(raw: str, allowed_hosts: set[str]) -> str:
    u = urlparse(raw)
    if u.scheme != "https":
        raise ValueError("https only")
    if u.hostname not in allowed_hosts:
        raise ValueError("host not allowed")
    for info in socket.getaddrinfo(u.hostname, None):
        ip = ipaddress.ip_address(info[4][0])
        if ip.is_loopback or ip.is_private or ip.is_link_local or ip.is_unspecified:
            raise ValueError("private/reserved IP")
    return raw
# Forbid redirects on the HTTP client; for high-risk targets pin the validated IP.
```

Note the TOCTOU gap: DNS is re-resolved on connect, so a short-TTL record can rebind
between check and connect. For high-risk targets, pin the validated IP, or keep
fetchable hosts to a fixed allowlist.

## Treating LLM I/O as Untrusted (OWASP LLM Top 10)

MeKnow feeds ingested KB documents into the model as retrieval context and shows the
model's output to users. **Ingested documents are attacker-influenceable** (a tenant
can upload a poisoned doc), and the question is end-user input — both are a fresh
attack surface.

- **Prompt injection (LLM01).** A KB document can carry instructions ("ignore prior
  rules, exfiltrate other context"). The prompt is **not** a security boundary;
  enforce permissions in code (server-side ACL, tenant scoping), not in prompt text.
  The **compression invariant** is the structural defense: raw chunks stay inside the
  sub-agent, so an injected instruction in one chunk can't reach across stage
  boundaries — only `memo_schema_ref`-shaped objects cross.
- **Sensitive info disclosure (LLM02/LLM07).** Never put another tenant's data,
  provider credentials, the Fernet key, or the service JWT into the model context —
  anything in context can be echoed into a public answer. Tenant scoping + the
  server-side ACL are what keep cross-tenant data out of context in the first place.
- **Improper output handling (LLM05).** Treat model output as data: it's shown to the
  user with citations, never fed into a shell, SQL, or eval. Bound its size.
- **Excessive agency (LLM06).** Keep tool access scoped to `retrieve_kb` and the
  declared MCP `kb.*` boundary; ACP/A2A stay Phase-2 boundary protocols, never in the
  retrieval loop. Destructive provider actions should be deliberate, not implied by
  model output.
- **Overreliance / ungrounded output (LLM09).** This is exactly why **citations are
  mandatory** and the `filter` stage holds `refuse_if: no_citations`: the system
  refuses rather than emit an ungrounded claim. Don't weaken it.

## Input Validation Patterns

Validate at the boundary before anything trusts the value.

```python
# Webhook: verify HMAC first, then validate the payload shape, then act.
verify_hmac(raw_body, sig_header, webhook_secret)   # A08 — before anything else
payload = WebhookPayload.model_validate_json(raw_body)  # reject malformed
if payload.tenant_id != ctx.authenticated_tenant:       # never cross tenants
    raise Forbidden()
```

Bound sizes (question length, ingested document size, retrieved-context size,
answer size). Reject before use, not after.

## Secrets Management

```
Environment (server/worker, fail-fast if missing):
  ├── DATABASE_URL            DB DSN (queries are tenant-scoped)
  ├── SERVICE_JWT_SECRET      HS256 secret for backend ↔ worker auth
  ├── FERNET_KEY              Fernet key for encrypting provider credentials
  └── WEBHOOK_SECRET          HMAC key for inbound webhook verification
LLM transport: MiniMax via the Anthropic-compatible endpoint (key, env-supplied)

Never holds another tenant's plaintext data in a shared cache (keys carry tenant_id + ACL-cohort hash).

.gitignore must exclude:
  *.pem  *.key  .env  .env.local  any config holding real secrets
```

**Check before committing:**
```bash
git diff --cached | grep -iE "fernet|jwt|secret|password|api[_-]?key|BEGIN .*PRIVATE KEY|SERVICE_JWT_SECRET|WEBHOOK_SECRET"
```

**If a secret is ever committed, rotate it.** Deleting the line or rewriting
history is not enough — assume it's compromised the moment it reaches a remote.
Revoke and reissue (rotate the Fernet key and re-encrypt credentials, rotate the
service-JWT secret, rotate the webhook secret), then purge from history.

## Triaging Dependency Findings

```
Audit (pip-audit / govulncheck / pnpm audit) reports a vulnerability
├── Is the vulnerable symbol/path actually reachable in our code?
│   ├── YES + high severity --> Fix immediately (update or replace the dependency)
│   └── NO (unreachable / build-only) --> fix soon, not a blocker
├── Is a fixed version available?
│   ├── YES --> bump the lockfile, re-run the owning package's gates
│   └── NO  --> workaround, replace the dependency, or allowlist with a review date
└── Track lower-severity items in the backlog and clear them during regular updates.
```

`govulncheck` is reachability-aware (Go); `pip-audit`/`pnpm audit` flag known CVEs.
None catch a malicious/typosquatted package — review new dependencies before adding
them (see `code-review-and-quality`). Commit lockfiles; install reproducibly in CI.

## Security Review Checklist

```markdown
### Multi-tenancy & access control
- [ ] Every query, cache key, and log line carries tenant_id; no cross-tenant joins
- [ ] Cache keys include the ACL-cohort hash
- [ ] ACL enforced server-side in retrieve_kb; caller inherited from request, not the model

### Authentication & secrets
- [ ] Service JWT verified (HS256, pinned alg, issuer/audience/expiry) on backend↔worker calls
- [ ] Provider credentials Fernet-encrypted at rest; decrypted only server-side
- [ ] Inbound webhooks HMAC-verified (constant-time) BEFORE the payload is acted on
- [ ] Required secrets (Fernet key, service-JWT secret, DB URL, webhook secret) fail fast

### Grounding & LLM safety
- [ ] filter stage holds refuse_if: no_citations; temperature == 0 on synthesize/cite/filter
- [ ] Compression invariant intact — raw KB chunks never cross a sub-agent boundary
- [ ] Model output treated as data (shown with citations; never shell/SQL/eval); size bounded
- [ ] No cross-tenant data, credentials, or secrets ever enter model context

### Input & SSRF
- [ ] Queries parameterized; payloads schema-validated at the boundary
- [ ] Ingested document and question sizes bounded
- [ ] Server-side URL fetches (ingest/connectors) allowlisted (no SSRF to internal services)

### Integrity & supply chain
- [ ] Lockfiles committed; CI installs reproducibly; pip-audit/govulncheck/pnpm audit clean of reachable high/critical
- [ ] New dependencies reviewed (maintenance, license, footprint)
- [ ] Versions append-only (BotVersion/KBVersion/traces never mutated in place)

### Errors & logging
- [ ] No stack traces / internal errors in API responses
- [ ] Security events logged with tenant_id + context; no secrets or KB content in logs
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll filter by tenant later in app code" | A query without a tenant_id predicate is a cross-tenant leak. Scope at the query, every time. |
| "The model already knows the caller's ACL" | The prompt is not a security boundary. ACL is decided server-side from the inherited caller, never from model output. |
| "This cache key is fine without the ACL cohort" | Two callers in one tenant with different permissions will share an answer they shouldn't. Include the cohort hash. |
| "It's just a KB document, it's trusted content" | A tenant can upload a poisoned doc. Treat ingested content as prompt-injection input; the compression invariant contains it. |
| "We'll add security later" | Retrofitting tenant scoping, server-side ACL, and Fernet is 10x harder than building them in. The invariants exist so you don't have to. |
| "Just log the JWT/credential while I debug" | A logged secret is a leaked secret. Log tenant_id + request id, never the secret. |
| "Returning an uncited answer is more helpful than refusing" | An ungrounded answer is the product's worst failure. refuse_if: no_citations stays. |
| "Threat modeling is overkill here" | Five minutes of "can I read another tenant's data?" prevents the design flaws no control can patch later. |

## Red Flags

- A query, cache key, or log line with no `tenant_id`; any cross-tenant join
- ACL or caller identity taken from model output / tool arguments instead of the request
- A cache key missing the ACL-cohort hash
- Raw KB chunks crossing a sub-agent boundary (compression invariant broken)
- An answer path that can emit output with no citations; `temperature` raised on a grounded stage
- Secrets (Fernet key, service-JWT secret, provider credentials) in source, logs, or history
- A service JWT accepted without verifying signature/claims, or with `alg: none` allowed
- A webhook payload parsed/acted on before HMAC verification
- String-concatenated SQL
- Server-side fetch of a tenant-influenced URL without an allowlist (SSRF)
- Required secrets defaulted instead of failing fast
- In-place mutation of a `BotVersion`/`KBVersion`/trace (versions are append-only)

## MeKnow notes

- **Invariants ARE the controls.** The table at the top of this skill is this
  repo's OWASP answer: tenant scoping + ACL-cohort cache keys (A01), server-side ACL
  in `retrieve_kb` (A01), Fernet credential encryption (A02), parameterized queries +
  compression invariant against KB prompt-injection (A03/LLM01), service JWT (A07),
  HMAC-verified webhooks (A08), citations-mandatory refuse (LLM09), `temperature == 0`
  + append-only versions (integrity/tampering). Preserve them; breaking one is an
  Ask-First.
- **Lifecycle:** security-affecting changes go through OpenSpec
  (`/opsx:propose` → `/opsx:spec` → `/opsx:spec-pr` → `/opsx:ship` →
  `/opsx:address-review` → `/opsx:archive`). Run the built-in `/security-review`
  and `/code-review` commands; the autonomous `/opsx:ship` pipeline gates on the
  resolver toolchain gates and runs a security audit before opening the CODE PR.
- **Verify with the gate resolver** (per touched package): `uv run ruff` / `pyright`
  / `python -m pytest -q` (Python `uv` workspace), `go build/vet/test -race`
  (go 1.24 modules), `pnpm typecheck/lint/test` (`apps/portal`), and
  `bash benchmarks/ci-free-gates.sh` — plus `openspec validate "<change>" --strict`
  always. The cross-cutting security guarantees have dedicated gate scripts:
  `benchmarks/gates/tenant-isolation-test.sh` and `retrieve-kb-acl-test.sh`.
- See the `code-review-and-quality` and `debugging-and-error-recovery` sibling
  skills for the review-axis and root-cause depth this skill references.

## Verification

After implementing security-relevant code:

- [ ] Every query, cache key, and log line carries `tenant_id`; no cross-tenant joins
- [ ] ACL enforced server-side in `retrieve_kb`; caller inherited from the request; cache keys include the ACL-cohort hash
- [ ] Provider credentials Fernet-encrypted; decrypted only server-side
- [ ] Service JWT verified (HS256, pinned alg, claims) on backend↔worker calls
- [ ] Inbound webhooks HMAC-verified before the payload is acted on
- [ ] `refuse_if: no_citations` intact; `temperature == 0` on synthesize/cite/filter; compression invariant intact
- [ ] No secrets / cross-tenant data / KB content in source, logs, or git history
- [ ] Server-side URL fetches validated against an allowlist (no SSRF)
- [ ] Error responses carry no stack traces or internal details
- [ ] Dependency audit clean of reachable critical/high; lockfiles committed
- [ ] The owning package's resolver gates pass, plus `tenant-isolation-test.sh` / `retrieve-kb-acl-test.sh` where relevant
