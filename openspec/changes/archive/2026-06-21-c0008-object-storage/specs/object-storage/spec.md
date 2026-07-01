## ADDED Requirements

### Requirement: S3-compatible object CRUD

The system SHALL provide an `ObjectStore` port that MUST support creating, reading,
inspecting, listing, and deleting objects identified by a bucket and key. Putting an
object then getting it SHALL return the stored bytes unchanged; heading an object
SHALL report its size, ETag, and last-modified time; listing SHALL be scoped to a
key prefix; and deleting an object SHALL remove it so a subsequent head or get
reports it gone.

#### Scenario: Put and get round-trips

- **WHEN** an object is put and then fetched by the same bucket and key
- **THEN** the stored bytes are returned unchanged

#### Scenario: List objects by prefix

- **WHEN** objects are listed by a key prefix
- **THEN** only objects whose key begins with that prefix are returned

#### Scenario: Head reports object metadata

- **WHEN** an object's metadata is queried
- **THEN** its size, ETag, and last-modified time are reported

#### Scenario: Delete removes the object

- **WHEN** an existing object is deleted
- **THEN** a subsequent head or get reports the object as gone

### Requirement: Presigned URLs for credential-free agent access

The system SHALL mint short-lived presigned GET and PUT URLs for an object, each
scoped by a caller-supplied time-to-live. The `ObjectStore` driver MUST be the only
component that holds raw store credentials; agents SHALL use the presigned URLs to
read and write objects without ever receiving store access keys, and the URLs MUST
stop granting access once their TTL elapses.

#### Scenario: Hub mints presigned GET and PUT URLs

- **WHEN** the server requests presigned GET and PUT URLs for an object with a TTL
- **THEN** both URLs are issued and the agent uses them to access the object without holding store credentials

#### Scenario: Presigned URL expires after its TTL

- **WHEN** a presigned URL is used after its time-to-live has elapsed
- **THEN** the access is rejected because the URL has expired

### Requirement: Endpoint-agnostic S3-compatible backends

The `ObjectStore` port SHALL behave identically across any S3-compatible backend,
and the backend MUST be selected from configuration (endpoint, region, bucket, and
credentials) so that AWS S3, MinIO, and Cloudflare R2 are interchangeable. The same
put, get, and list calls SHALL produce the same behavior regardless of which backend
is configured.

#### Scenario: Same calls work on any S3-compatible endpoint

- **WHEN** the store is configured against AWS S3, MinIO, or Cloudflare R2 and the same put, get, and list calls are made
- **THEN** the observable behavior is identical across the backends

### Requirement: Multipart upload of large objects

The `ObjectStore` port SHALL support uploading a large object as an ordered sequence
of parts via a multipart operation, and the parts MUST assemble into a single object
whose final ETag is returned. A multipart upload that completes successfully SHALL
yield a readable object equivalent to the concatenation of its parts.

#### Scenario: Multipart upload assembles parts into one object

- **WHEN** a large object split into ordered parts is uploaded via multipart
- **THEN** the parts assemble into one object and a final ETag is returned

### Requirement: Per-driver dependency isolation

Each storage backend SHALL be implemented as a separate driver under
`server/storage` (for example `server/storage/s3`, `server/storage/minio`,
`server/storage/r2`, and `server/storage/fs`), and a build MUST link only the SDK of
the driver it wires in. Consumers SHALL depend on the `ObjectStore` port in
`shared/ports` rather than on any concrete driver, and the filesystem driver MUST add
no external storage SDK dependency.

#### Scenario: A build links only the wired driver

- **WHEN** a build wires in a single storage driver
- **THEN** only that driver's SDK is linked and the other drivers' dependencies are excluded

#### Scenario: Consumers depend on the port, not a driver

- **WHEN** a consumer uses object storage
- **THEN** it imports the `ObjectStore` port from `shared/ports` and works with any driver without changing callers
