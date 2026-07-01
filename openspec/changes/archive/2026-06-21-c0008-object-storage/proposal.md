## Why

The redesign needs a durable, online-backed place to keep session workspaces and
agent artifacts so that work survives across sandbox runs and can be shared and
resumed. That backend must be **S3-compatible** so operators can point it at the
infrastructure they already run — AWS S3 in the cloud, MinIO on-prem, Cloudflare R2
at the edge, or a plain filesystem for local development — without changing any
consuming code. Today there is no object-storage abstraction at all: nothing in the
codebase persists workspace state or artifacts to a remote store.

This change introduces a **pluggable, S3-compatible object store** behind a single
`ObjectStore` port. It is the only component that holds raw store credentials;
agents access objects exclusively through short-lived **presigned URLs**, so store
keys never leave the server.

## What Changes

- A new **object-storage** capability: an `ObjectStore` port in `shared/ports`
  covering object CRUD (PutObject/GetObject/HeadObject/ListObjects/DeleteObject),
  presigned GET/PUT URL minting, and multipart upload of large objects.
- Drivers under `server/storage/{s3,minio,r2,fs}` implementing the port against AWS
  S3, MinIO, Cloudflare R2, and a local filesystem backend.
- Endpoint-agnostic configuration (endpoint + region + bucket + credentials) so the
  same port works identically across every backend.
- Per-driver SDK isolation so a build links only the driver it wires in
  (`server/storage/<driver>`), keeping the dependency footprint minimal.

## Capabilities

### New Capabilities
- `object-storage`: an S3-compatible object store behind the `ObjectStore` port with
  pluggable drivers (`s3`/`minio`/`r2`/`fs`), presigned credential-free access, and
  multipart upload.

## Impact

- **Sequenced after `c0002-repo-restructure`**: the port lands in `shared/ports` and
  drivers under `server/storage/{s3,minio,r2,fs}`, the module layout established by
  that change.
- **Consumed by `c0010-session-workspaces`** (the `WorkspaceManager`/`WorkspaceFS`
  layers sync to this store) and by **`c0013` artifacts** (artifact persistence and
  retrieval).
- New driver-gated dependencies: an S3-compatible SDK for the `s3`/`minio`/`r2`
  drivers; the `fs` driver keeps zero new external deps.
- Behaviors are pinned by the e2e scenarios `STORE-01` (put/get round-trip),
  `STORE-02` (list by prefix), `STORE-03` (head metadata), `STORE-04` (delete),
  `STORE-05` (presigned GET/PUT URLs), `STORE-06` (S3-compatible across
  AWS/MinIO/R2 endpoints), and `STORE-07` (multipart large-object upload) in
  `tests/e2e/23_workspace_storage_test.go`.
