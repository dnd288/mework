## 1. ObjectStore port

- [ ] 1.1 Define the `ObjectStore` port in `shared/ports` with `PutObject`/`GetObject`/`HeadObject`/`ListObjects`/`DeleteObject`
- [ ] 1.2 Add `PresignGetURL`/`PresignPutURL` (each taking a TTL) and `PutMultipart` to the port
- [ ] 1.3 Define the supporting value types (object ref, object info with size/etag/last-modified, put options)

## 2. S3-family drivers

- [ ] 2.1 Implement the `s3` driver in `server/storage/s3` against an S3-compatible endpoint (endpoint + region + bucket + credentials)
- [ ] 2.2 Implement the `minio` driver in `server/storage/minio`
- [ ] 2.3 Implement the `r2` driver in `server/storage/r2`
- [ ] 2.4 Implement presigned GET/PUT URL minting and multipart upload in the S3-family drivers

## 3. Filesystem driver

- [ ] 3.1 Implement the `fs` driver in `server/storage/fs` backing the same port with the local filesystem (no external SDK dependency)
- [ ] 3.2 Emulate head metadata, prefix listing, and presigned/multipart semantics for development and tests

## 4. Selection & isolation

- [ ] 4.1 Select the driver from configuration (endpoint/region/bucket/credentials); default and override rules
- [ ] 4.2 Keep each driver's SDK in its own subpackage so a build links only the wired driver's dependency

## 5. Validate

- [ ] 5.1 Tests: object CRUD round-trips, prefix listing, head metadata, delete, presigned GET/PUT URLs, multipart upload, and identical behavior across the S3-family drivers
- [ ] 5.2 `openspec validate c0008-object-storage --type change --strict`
- [ ] 5.3 e2e pointer: flip `tests/e2e/23_workspace_storage_test.go` from Skip to Green for STORE-01..07 (S3-compatible put/get, list-by-prefix, head, delete, presigned GET/PUT, endpoint-agnostic AWS/MinIO/R2, multipart). The same `ObjectStore` port is the dependency for WS-01..09, SHARE-01..06, and ARTIFACT-* — landing this change unblocks those.
