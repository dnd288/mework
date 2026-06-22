## Context

The hub must distribute agents and constrain them, GitHub-Actions style. Current
state: `profiles` is static per-account config (`server/catalog`) with no
versioning, distribution, or permissions. The `message-bus` change provides the
topic/publish substrate this builds on for dispatch.

## Goals / Non-Goals

**Goals:**
- Make agents first-class, versioned, immutable, pullable artifacts.
- Support multiple artifact forms (definition/manifest and image reference).
- Dispatch an agent version to a target runner/session via the bus.
- Attach an explicit, least-privilege permission grant to every dispatch.

**Non-Goals:**
- The runner's pull/run/report loop and enrollment — that is `agent-runner`.
- How a sandbox materializes/runs the artifact — that is `sandbox-runtime`.
- The transport itself — that is `message-bus`.

## Decisions

- **Immutable versions, moving pointers.** Versions are content-addressable/immutable;
  `latest` (and named channels) are pointers resolved at dispatch.
- **Type-agnostic artifacts.** Record a `form` (`definition` | `image`) plus the
  payload/reference; the consumer chooses materialization. This mirrors GitHub
  Actions' composite-vs-container split and avoids locking the design to one form.
- **Dispatch = publish.** Dispatch does not push bytes; it publishes a small
  dispatch message (agent ref + grant) to the target topic. The runner pulls the
  artifact lazily — keeps the bus light and supports large/image artifacts.
- **Grants are explicit and least-privilege.** A dispatch with no grant for an
  operation means that operation is denied. Grants are scoped per-run, not
  per-identity, so the same runner can be highly privileged for one dispatch and
  minimal for another.
- **Profiles → agents migration.** Existing `profiles` map onto `definition`-form
  agents; provide a migration so current config is not lost.

## Risks / Trade-offs

- **Artifact storage.** Image-form agents imply a registry/storage concern; v1 can
  store a reference to an external registry rather than hosting bytes.
- **Permission model scope creep.** Risk of an over-rich policy language; start with
  a small, enumerable operation set and expand deliberately.
- **Version sprawl / GC.** Immutable versions accumulate; needs a retention/GC
  policy (out of scope for v1, noted).
- **Grant forgery.** Grants must be integrity-protected (signed/sealed) so a runner
  cannot widen its own scope — reuse `server/platform/{token,secret}` primitives.
