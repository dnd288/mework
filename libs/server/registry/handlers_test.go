package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"mework/libs/server/auth"
)

// withTenantContext builds a request whose context carries the auth middleware's
// resolved account and tenant, mirroring what the production PAT middleware injects.
// The HTTP handlers route their service calls through callerTenant(r), so this is the
// only seam that decides which tenant the request is scoped to.
func withTenantContext(method, target, accountID, tenantID string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	ctx := context.WithValue(req.Context(), auth.AccountIDKey, accountID)
	ctx = context.WithValue(ctx, auth.TenantIDKey, tenantID)
	return req.WithContext(ctx)
}

// TestListRuntimes_CrossTenantIsolation proves the production HTTP path enforces the
// tenancy boundary: a credential scoped to tenant A, hitting ListRuntimes, never sees a
// runtime owned by tenant B.
func TestListRuntimes_CrossTenantIsolation(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)

	tenantA, err := svc.RegisterTenant(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("RegisterTenant(tenant-a): %v", err)
	}
	tenantB, err := svc.RegisterTenant(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("RegisterTenant(tenant-b): %v", err)
	}

	// Seed a runtime that lives ONLY under tenant B.
	const tenantBCode = "tenant-b-secret-rt"
	seedRuntime(t, ctx, svc.pool, tenantB.ID, accountID, tenantBCode)

	handlers := NewHandlers(svc, nil)

	tests := []struct {
		name       string
		tenantID   string
		wantLeaked bool
	}{
		{name: "tenant A cannot see tenant B's runtime", tenantID: tenantA.ID, wantLeaked: false},
		{name: "tenant B sees its own runtime", tenantID: tenantB.ID, wantLeaked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := withTenantContext(http.MethodGet, "/api/v1/runtimes", accountID, tt.tenantID)
			rec := httptest.NewRecorder()

			handlers.ListRuntimes(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("ListRuntimes status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
			}

			leaked := strings.Contains(rec.Body.String(), tenantBCode)
			if leaked != tt.wantLeaked {
				t.Errorf("body leaked=%v, want %v; cross-tenant runtime %q must not appear for tenant %q (body: %s)",
					leaked, tt.wantLeaked, tenantBCode, tt.tenantID, rec.Body.String())
			}
		})
	}
}

// TestDeleteRuntime_CrossTenantIsolation proves the production HTTP path denies a
// cross-tenant delete: a credential scoped to tenant A cannot delete tenant B's runtime
// — the handler returns 404 (the runtime is invisible, not merely forbidden) and the
// runtime still exists. The owning tenant can delete it (204).
func TestDeleteRuntime_CrossTenantIsolation(t *testing.T) {
	ctx, svc, accountID := newTenancyTestService(t)

	tenantA, err := svc.RegisterTenant(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("RegisterTenant(tenant-a): %v", err)
	}
	tenantB, err := svc.RegisterTenant(ctx, "tenant-b")
	if err != nil {
		t.Fatalf("RegisterTenant(tenant-b): %v", err)
	}

	runtimeID := seedRuntime(t, ctx, svc.pool, tenantB.ID, accountID, "tenant-b-only")
	handlers := NewHandlers(svc, nil)

	deleteReq := func(tenantID, id string) *httptest.ResponseRecorder {
		req := withTenantContext(http.MethodDelete, "/api/v1/runtimes/"+id, accountID, tenantID)
		chiCtx := chi.NewRouteContext()
		chiCtx.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
		rec := httptest.NewRecorder()
		handlers.DeleteRuntime(rec, req)
		return rec
	}

	// Tenant A must NOT be able to delete tenant B's runtime: 404, and it survives.
	t.Run("tenant A cannot delete tenant B's runtime", func(t *testing.T) {
		rec := deleteReq(tenantA.ID, runtimeID)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant DeleteRuntime status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
		}

		// The runtime must still exist under tenant B.
		runtimes, err := svc.ListRunners(ctx, *tenantB, accountID)
		if err != nil {
			t.Fatalf("ListRunners(tenant-b): %v", err)
		}
		found := false
		for _, rt := range runtimes {
			if rt.ID == runtimeID {
				found = true
			}
		}
		if !found {
			t.Errorf("runtime %q was removed by a cross-tenant delete; it must survive", runtimeID)
		}
	})

	// The owning tenant CAN delete it.
	t.Run("tenant B can delete its own runtime", func(t *testing.T) {
		rec := deleteReq(tenantB.ID, runtimeID)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("owning-tenant DeleteRuntime status = %d, want 204 (body: %s)", rec.Code, rec.Body.String())
		}
	})
}
