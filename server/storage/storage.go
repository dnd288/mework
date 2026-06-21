// Package storage defines the object-store interface for server-side blob
// storage and the ArtifactStore that persists run outputs over the object
// store with checksum integrity verification.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"mework/shared/core"
	"mework/shared/ports"
)

// Store is the server's object storage interface.
type Store interface {
	Put(ctx context.Context, ref core.ObjectRef, reader io.Reader) error
	Get(ctx context.Context, ref core.ObjectRef) (io.ReadCloser, error)
	Delete(ctx context.Context, ref core.ObjectRef) error
	List(ctx context.Context, prefix string) ([]core.ObjectInfo, error)
	PresignGetURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, error)
	PresignPutURL(ctx context.Context, ref core.ObjectRef, ttl time.Duration) (string, error)
}

// ArtifactStore persists and serves run outputs/artifacts over an ObjectStore
// with per-tenant scoping, SHA256 checksum integrity, and presigned URLs for
// sandbox agent access (the runner never holds store credentials).
type ArtifactStore struct {
	store ports.ObjectStore
}

// NewArtifactStore creates an ArtifactStore backed by the given ObjectStore.
func NewArtifactStore(store ports.ObjectStore) *ArtifactStore {
	return &ArtifactStore{store: store}
}

// compile-time check: *ArtifactStore implements ports.ArtifactStore
var _ ports.ArtifactStore = (*ArtifactStore)(nil)

// artifactKey builds the object-key for a run artifact.
// Layout: tenants/{tenant}/runs/{runID}/artifacts/{name}
func artifactKey(tenant core.TenantID, runID, name string) core.ObjectRef {
	return core.ObjectRef{
		Key: fmt.Sprintf("tenants/%s/runs/%s/artifacts/%s", tenant, runID, name),
	}
}

// checksumKey builds the object-key for an artifact's SHA256 checksum file.
func checksumKey(tenant core.TenantID, runID, name string) core.ObjectRef {
	return core.ObjectRef{
		Key: fmt.Sprintf("tenants/%s/runs/%s/artifacts/%s.sha256", tenant, runID, name),
	}
}

// artifactPrefix returns the prefix for listing all artifacts of a run.
func artifactPrefix(tenant core.TenantID, runID string) string {
	return fmt.Sprintf("tenants/%s/runs/%s/artifacts/", tenant, runID)
}

// Put stores content as a run artifact and records its SHA256 checksum.
func (a *ArtifactStore) Put(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, content []byte) error {
	if ref.RunID == "" || ref.Name == "" {
		return fmt.Errorf("artifact ref requires run_id and name")
	}

	// Compute SHA256 checksum of the content.
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	// Store the content.
	objRef := artifactKey(tenant, ref.RunID, ref.Name)
	if err := a.store.Put(ctx, objRef, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("store artifact content: %w", err)
	}

	// Store the checksum alongside the content for integrity verification.
	csRef := checksumKey(tenant, ref.RunID, ref.Name)
	if err := a.store.Put(ctx, csRef, bytes.NewReader([]byte(checksum))); err != nil {
		return fmt.Errorf("store artifact checksum: %w", err)
	}

	return nil
}

// Get retrieves a run artifact and verifies its SHA256 checksum.
// Returns an integrity error if the checksum does not match.
func (a *ArtifactStore) Get(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef) ([]byte, error) {
	if ref.RunID == "" || ref.Name == "" {
		return nil, fmt.Errorf("artifact ref requires run_id and name")
	}

	objRef := artifactKey(tenant, ref.RunID, ref.Name)

	// Read the content.
	reader, err := a.store.Get(ctx, objRef)
	if err != nil {
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}

	// Read the stored checksum and verify integrity.
	csRef := checksumKey(tenant, ref.RunID, ref.Name)
	csReader, csErr := a.store.Get(ctx, csRef)
	if csErr == nil {
		defer csReader.Close()
		storedSumBytes, _ := io.ReadAll(csReader)
		storedChecksum := strings.TrimSpace(string(storedSumBytes))

		actualSum := sha256.Sum256(content)
		actualChecksum := hex.EncodeToString(actualSum[:])

		if actualChecksum != storedChecksum {
			return nil, fmt.Errorf("artifact integrity check failed: checksum mismatch for %s/%s", ref.RunID, ref.Name)
		}
	}

	return content, nil
}

// List returns metadata for all artifacts stored under the given run.
func (a *ArtifactStore) List(ctx context.Context, tenant core.TenantID, runID string) ([]core.ArtifactInfo, error) {
	prefix := artifactPrefix(tenant, runID)

	objects, err := a.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	var infos []core.ArtifactInfo
	for _, obj := range objects {
		// Skip checksum entries (they end with .sha256).
		if strings.HasSuffix(obj.Ref.Key, ".sha256") {
			continue
		}

		// Extract the artifact name from the key.
		name := artifactNameFromKey(obj.Ref.Key)

		infos = append(infos, core.ArtifactInfo{
			Ref:  core.ArtifactRef{RunID: runID, Name: name},
			Size: obj.Size,
		})
	}

	return infos, nil
}

// PresignGetURL returns a presigned GET URL so a sandbox agent can read an
// artifact without holding store credentials.
func (a *ArtifactStore) PresignGetURL(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, ttl time.Duration) (string, error) {
	objRef := artifactKey(tenant, ref.RunID, ref.Name)
	url, err := a.store.PresignGetURL(ctx, objRef, ttl)
	if err != nil {
		return "", fmt.Errorf("presign get url: %w", err)
	}
	return url, nil
}

// PresignPutURL returns a presigned PUT URL so a sandbox agent can write an
// artifact without holding store credentials.
func (a *ArtifactStore) PresignPutURL(ctx context.Context, tenant core.TenantID, ref core.ArtifactRef, ttl time.Duration) (string, error) {
	objRef := artifactKey(tenant, ref.RunID, ref.Name)
	url, err := a.store.PresignPutURL(ctx, objRef, ttl)
	if err != nil {
		return "", fmt.Errorf("presign put url: %w", err)
	}
	return url, nil
}

// artifactNameFromKey extracts the artifact name from a key.
// Key format: tenants/{tenant}/runs/{runID}/artifacts/{name}
func artifactNameFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 4 {
		return key
	}
	return parts[len(parts)-1]
}
