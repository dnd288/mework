package catalog

import (
	"context"
	"errors"
	"testing"

	"mework/libs/sandbox"
)

// TestPrebuilt_PublishAndResolve covers the prebuilt-definition layer that wraps
// the agent catalog Service: definitions are published as immutable catalog
// artifacts, republishing a version with different content is rejected, and a
// moving "latest" pointer resolves to the concrete latest version.
//
// DB-backed: skips without TEST_DATABASE_URL.
func TestPrebuilt_PublishAndResolve(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	pool, cleanup := setupAgentTestDB(t, dsn)
	defer cleanup()
	svc := NewService(pool)
	ctx := context.Background()

	t.Run("Publish a prebuilt definition", func(t *testing.T) {
		meta := sandbox.SandboxBundleMetadata{
			Name:    "local-claude",
			Version: "1.0.0",
			Engine:  "local",
			Backend: "claude",
		}
		if _, err := svc.PublishDefinition(ctx, meta); err != nil {
			t.Fatalf("PublishDefinition: %v", err)
		}

		got, err := svc.ResolveDefinition(ctx, "local-claude", "1.0.0")
		if err != nil {
			t.Fatalf("ResolveDefinition: %v", err)
		}
		if got.Name != meta.Name {
			t.Errorf("Name = %q, want %q", got.Name, meta.Name)
		}
		if got.Version != meta.Version {
			t.Errorf("Version = %q, want %q", got.Version, meta.Version)
		}
		if got.Engine != meta.Engine {
			t.Errorf("Engine = %q, want %q", got.Engine, meta.Engine)
		}
		if got.Backend != meta.Backend {
			t.Errorf("Backend = %q, want %q", got.Backend, meta.Backend)
		}
	})

	t.Run("Republishing an existing version is rejected", func(t *testing.T) {
		base := sandbox.SandboxBundleMetadata{
			Name:    "republish-claude",
			Version: "1.0.0",
			Engine:  "local",
			Backend: "claude",
		}
		if _, err := svc.PublishDefinition(ctx, base); err != nil {
			t.Fatalf("first PublishDefinition: %v", err)
		}

		// Same name@version but DIFFERENT content (different backend).
		different := base
		different.Backend = "codex"
		_, err := svc.PublishDefinition(ctx, different)
		if !errors.Is(err, ErrVersionAlreadyExists) {
			t.Fatalf("second PublishDefinition: got %v, want ErrVersionAlreadyExists", err)
		}

		// The stored content must be unchanged (still the first one).
		got, err := svc.ResolveDefinition(ctx, "republish-claude", "1.0.0")
		if err != nil {
			t.Fatalf("ResolveDefinition after rejected republish: %v", err)
		}
		if got.Backend != base.Backend {
			t.Errorf("Backend = %q, want unchanged %q", got.Backend, base.Backend)
		}
	})

	t.Run("Resolve a moving pointer", func(t *testing.T) {
		v100 := sandbox.SandboxBundleMetadata{
			Name:    "moving-claude",
			Version: "1.0.0",
			Engine:  "local",
			Backend: "claude",
		}
		if _, err := svc.PublishDefinition(ctx, v100); err != nil {
			t.Fatalf("publish 1.0.0: %v", err)
		}

		v110 := v100
		v110.Version = "1.1.0"
		if _, err := svc.PublishDefinition(ctx, v110); err != nil {
			t.Fatalf("publish 1.1.0: %v", err)
		}

		got, err := svc.ResolveDefinition(ctx, "moving-claude", "latest")
		if err != nil {
			t.Fatalf("ResolveDefinition latest: %v", err)
		}
		if got.Version != "1.1.0" {
			t.Errorf("latest Version = %q, want %q", got.Version, "1.1.0")
		}
	})
}
