## Context

Session workspaces and agent artifacts need a durable, online-backed home so work
survives across sandbox runs and can be shared and resumed. Operators run different
storage infrastructure — AWS S3, MinIO, Cloudflare R2, or just a local filesystem in
development — but they all speak (or can speak) the S3 object model: buckets, keys,
prefixes, presigned URLs, multipart upload. The codebase has no object-storage seam
today, so this change defines one `ObjectStore` port in `shared/ports` and a set of
drivers under `server/storage`, each implementing the port against one backend.

## Goals / Non-Goals

**Goals:**
- A single `ObjectStore` port that consumers depend on instead of any concrete SDK.
- Object CRUD: put, get, head (metadata), list by prefix, delete.
- Presigned GET/PUT URLs with a TTL so agents read/write objects without ever
  holding store credentials.
- Endpoint-agnostic configuration so the same port behaves identically on AWS S3,
  MinIO, and Cloudflare R2.
- Multipart upload so large objects can be streamed in parts.
- Per-driver dependency isolation: a build links only the SDK of the driver it wires.

**Non-Goals:**
- Workspace mount/sync/scope semantics (that is `c0010-session-workspaces`).
- Artifact lifecycle and retention (that is `c0013` artifacts).
- A bucket/lifecycle provisioning or migration tool — operators provision buckets.
- Choosing one canonical backend; the port keeps all of them interchangeable.

## Decisions

- **The `ObjectStore` port (in `shared/ports`).** It exposes
  `PutObject`/`GetObject`/`HeadObject`/`ListObjects`/`DeleteObject` for object CRUD,
  `PresignGetURL`/`PresignPutURL` (each taking a TTL) for credential-free agent
  access, and `PutMultipart` for streaming a large object as ordered parts and
  returning a final ETag. `HeadObject` returns size, ETag, and last-modified;
  `ListObjects` is prefix-scoped (the tenant/session boundary).
- **Drivers under `server/storage/{s3,minio,r2,fs}`.** Each driver implements the
  port against one backend. `s3`/`minio`/`r2` target an S3-compatible endpoint
  (endpoint + region + bucket + credentials); `fs` backs the same contract with the
  local filesystem for development and tests.
- **Endpoint-agnostic config.** Backend selection and connection details
  (endpoint/region/bucket/credentials) come from configuration, so the same
  put/get/list calls produce identical behavior across AWS S3, MinIO, and R2.
- **Per-driver SDK isolation.** Each driver lives in its own subpackage so a build
  links only the SDK of the driver it wires in; the `fs` driver adds no external
  dependency. Consumers import the port, not a driver.
- **The agent never holds raw store credentials.** The `ObjectStore` driver is the
  only component that holds store keys; agents receive short-lived presigned GET/PUT
  URLs (or a hub-proxied path) and use those, so credentials stay server-side.

## Risks / Trade-offs

- **S3-compatibility gaps across backends.** MinIO and R2 are S3-compatible but not
  byte-identical to AWS (e.g. region quirks, multipart minimum part sizes, presign
  signature versions); mitigate by exercising the same contract tests against each
  driver and keeping the port to the well-supported common subset.
- **Presigned-URL TTL trade-off.** Short TTLs limit exposure but can expire
  mid-transfer for large objects; the caller chooses the TTL per operation and may
  re-mint, with multipart for very large uploads.
- **Driver isolation vs. shared code.** Keeping SDKs per-driver avoids linking unused
  dependencies but risks duplicated glue; share only backend-neutral helpers in
  `server/storage` and keep SDK-specific code inside each driver subpackage.
- **fs driver fidelity.** The filesystem driver must emulate ETag/last-modified and
  presigned semantics convincingly enough for development without overpromising
  production guarantees; it is documented as a dev/test backend.
