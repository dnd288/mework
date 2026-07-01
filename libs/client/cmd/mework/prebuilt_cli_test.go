package main

import (
	"context"
	"strings"
	"testing"

	"mework/libs/sandbox"
	"mework/libs/shared/core"
)

// fakeCatalog is a read-only stand-in for the agent-catalog resolver. It serves
// a fixed list of prebuilt definitions and records whether any mutating call was
// attempted (there must be none from the read-only CLI surface).
type fakeCatalog struct {
	defs []sandbox.SandboxBundleMetadata
}

func (f *fakeCatalog) ListDefinitions(ctx context.Context) ([]sandbox.SandboxBundleMetadata, error) {
	return f.defs, nil
}

// fakeSessions is a read-only stand-in for the session client. It returns
// sessions filtered to the requested tenant, mirroring the tenant-scoped List
// contract of the interactive session manager.
type fakeSessions struct {
	all []core.SessionInfo
}

func (f *fakeSessions) ListSessions(ctx context.Context, tenant core.TenantID) ([]core.SessionInfo, error) {
	var out []core.SessionInfo
	for _, s := range f.all {
		if s.Tenant == tenant {
			out = append(out, s)
		}
	}
	return out, nil
}

// TestAgentList_PrintsPrebuiltDefinitions verifies that `agent list` prints the
// available prebuilt definitions (name and version) from the catalog resolver
// in a read-only fashion.
//
// RED: RenderAgentList is not implemented yet — this fails to compile.
func TestAgentList_PrintsPrebuiltDefinitions(t *testing.T) {
	tests := []struct {
		name        string
		defs        []sandbox.SandboxBundleMetadata
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "three starter definitions",
			defs: []sandbox.SandboxBundleMetadata{
				{Name: "local-claude", Version: "1.0.0", Engine: "local", Backend: "claude"},
				{Name: "docker-claude", Version: "1.0.0", Engine: "docker", Backend: "claude", Image: "mework/claude:1.0.0"},
				{Name: "codex-docker", Version: "1.0.0", Engine: "docker", Backend: "codex", Image: "mework/codex:1.0.0"},
			},
			wantContain: []string{"local-claude", "docker-claude", "codex-docker", "1.0.0"},
		},
		{
			name:        "empty catalog",
			defs:        nil,
			wantContain: nil,
			wantAbsent:  []string{"local-claude"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := &fakeCatalog{defs: tt.defs}
			var buf strings.Builder

			if err := RenderAgentList(context.Background(), &buf, cat); err != nil {
				t.Fatalf("RenderAgentList: %v", err)
			}

			got := buf.String()
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\n--- output ---\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output unexpectedly contains %q\n--- output ---\n%s", absent, got)
				}
			}
		})
	}
}

// TestSessionList_FiltersToCallerTenant verifies that `session list` prints only
// the sessions belonging to the caller's tenant, identified by session ID, and
// does not leak another tenant's sessions.
//
// RED: RenderSessionList is not implemented yet — this fails to compile.
func TestSessionList_FiltersToCallerTenant(t *testing.T) {
	sessions := []core.SessionInfo{
		{ID: "sess-a1", Tenant: "tenant-a", Status: core.SessionActive, Owner: "acct-1"},
		{ID: "sess-a2", Tenant: "tenant-a", Status: core.SessionIdle, Owner: "acct-1"},
		{ID: "sess-b1", Tenant: "tenant-b", Status: core.SessionActive, Owner: "acct-2"},
	}

	tests := []struct {
		name        string
		tenant      core.TenantID
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "tenant A sees only its sessions",
			tenant:      "tenant-a",
			wantContain: []string{"sess-a1", "sess-a2"},
			wantAbsent:  []string{"sess-b1"},
		},
		{
			name:        "tenant B sees only its sessions",
			tenant:      "tenant-b",
			wantContain: []string{"sess-b1"},
			wantAbsent:  []string{"sess-a1", "sess-a2"},
		},
		{
			name:        "unknown tenant sees nothing",
			tenant:      "tenant-c",
			wantContain: nil,
			wantAbsent:  []string{"sess-a1", "sess-a2", "sess-b1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &fakeSessions{all: sessions}
			var buf strings.Builder

			if err := RenderSessionList(context.Background(), &buf, sc, tt.tenant); err != nil {
				t.Fatalf("RenderSessionList: %v", err)
			}

			got := buf.String()
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\n--- output ---\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output leaked %q from another tenant\n--- output ---\n%s", absent, got)
				}
			}
		})
	}
}
