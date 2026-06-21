// RED: Grant enforcement middleware tests — these fail because GrantMiddleware,
// RequireOperation, and GetGrant are not yet implemented in the middleware
// package. See .handoff/c0004-agent-catalog/tasks/03-auth-integration-e2e.md.
package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mework/libs/server/middleware"
	"mework/libs/shared/grant"
)

// marshalGrant serialises a Grant to a JSON string suitable for the X-Grant header.
func marshalGrant(t *testing.T, g *grant.Grant) string {
	t.Helper()
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal grant: %v", err)
	}
	return string(data)
}

func TestGrantMiddleware(t *testing.T) {
	key := []byte("test-grant-key-32bytes!!!!")

	// validGrant permits OpPullAgent with a valid signature.
	validGrant, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, key)
	if err != nil {
		t.Fatalf("NewGrant(valid): %v", err)
	}

	// readGrant permits only OpRepoRead (no OpPullAgent).
	readGrant, err := grant.NewGrant([]grant.Operation{grant.OpRepoRead}, key)
	if err != nil {
		t.Fatalf("NewGrant(read): %v", err)
	}

	// tamperedGrant was created with OpPullAgent, then ops were widened after signing.
	tamperedGrant, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, key)
	if err != nil {
		t.Fatalf("NewGrant(tampered): %v", err)
	}
	tamperedGrant.Ops = append(tamperedGrant.Ops, grant.OpSpawn)

	tests := []struct {
		name        string
		grantObj    *grant.Grant // nil means no X-Grant header
		wantOK      bool         // true → 200, false → 403
		description string
	}{
		{
			name:        "allows within scope",
			grantObj:    validGrant,
			wantOK:      true,
			description: "GrantMiddleware reads a grant permitting OpPullAgent from the header, injects it; RequireOperation(OpPullAgent) permits the call",
		},
		{
			name:        "denies outside scope",
			grantObj:    readGrant,
			wantOK:      false,
			description: "GrantMiddleware injects a grant that only permits OpRepoRead; RequireOperation(OpPullAgent) denies because the op is not in the grant",
		},
		{
			name:        "no grant denies",
			grantObj:    nil,
			wantOK:      false,
			description: "No X-Grant header present; RequireOperation finds no grant in context and returns 403",
		},
		{
			name:        "tampered grant denies",
			grantObj:    tamperedGrant,
			wantOK:      false,
			description: "GrantMiddleware reads a grant whose ops were widened after signing; VerifyGrant detects the mismatch and returns 403 before RequireOperation runs (GRANT-01)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build handler chain: GrantMiddleware → RequireOperation(OpPullAgent) → handler.
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := middleware.RequireOperation(grant.OpPullAgent)(inner)
			handler = middleware.GrantMiddleware(key)(handler)

			req := httptest.NewRequest("GET", "/api/v1/agents/test-agent/versions/1.0.0/pull", nil)
			if tt.grantObj != nil {
				req.Header.Set("X-Grant", marshalGrant(t, tt.grantObj))
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if tt.wantOK && rec.Code != http.StatusOK {
				t.Errorf("expected 200 OK, got %d; body: %s", rec.Code, rec.Body.String())
			}
			if !tt.wantOK && rec.Code != http.StatusForbidden {
				t.Errorf("expected 403 Forbidden, got %d; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGrantMiddleware_GetGrant(t *testing.T) {
	// This test verifies that GetGrant retrieves a grant correctly from the
	// context after GrantMiddleware has injected it.
	key := []byte("test-grant-key-getgrant")

	g, err := grant.NewGrant([]grant.Operation{grant.OpPullAgent}, key)
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	var gotGrant *grant.Grant
	var gotOK bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotGrant, gotOK = middleware.GetGrant(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.GrantMiddleware(key)(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Grant", marshalGrant(t, g))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !gotOK {
		t.Fatal("GetGrant returned ok=false after GrantMiddleware injected the grant")
	}
	if gotGrant == nil {
		t.Fatal("GetGrant returned nil grant")
	}
	if !gotGrant.Permits(grant.OpPullAgent) {
		t.Error("injected grant should permit OpPullAgent")
	}
}
