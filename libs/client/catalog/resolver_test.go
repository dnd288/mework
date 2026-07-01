package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mework/libs/client/runner"
	"mework/libs/sandbox"
)

// compile-time assertion that the resolver satisfies the runner contract.
var _ runner.DefinitionResolver = (*HTTPDefinitionResolver)(nil)

// agentVersionWire mirrors the server's catalog.AgentVersion JSON shape closely
// enough for the resolver to decode. The Payload is a []byte so Go marshals it
// as base64 exactly the way the server does (json.Marshal of []byte → base64),
// and it carries the JSON-encoded SandboxBundleMetadata.
type agentVersionWire struct {
	ID      string `json:"id"`
	AgentID string `json:"agent_id"`
	Version string `json:"version"`
	Form    string `json:"form"`
	Payload []byte `json:"payload,omitempty"`
}

// newCatalogServer stands up an httptest.Server that records the last request's
// path and version query, and serves the canned AgentVersion (or 404 for the
// notFound name).
func newCatalogServer(t *testing.T, meta sandbox.SandboxBundleMetadata, notFoundName string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}

	payload, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.path = r.URL.Path
		rec.version = r.URL.Query().Get("version")

		// Path is /api/v1/agents/{name}; extract the trailing name segment.
		const prefix = "/api/v1/agents/"
		name := ""
		if len(r.URL.Path) > len(prefix) {
			name = r.URL.Path[len(prefix):]
		}
		rec.name = name

		if name == notFoundName {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agentVersionWire{
			ID:      "ver-1",
			AgentID: "agent-1",
			Version: meta.Version,
			Form:    "definition",
			Payload: payload,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

type recordedRequest struct {
	path    string
	name    string
	version string
}

func TestHTTPDefinitionResolver_ResolveDefinition(t *testing.T) {
	wantMeta := sandbox.SandboxBundleMetadata{
		Name:    "code-fixer",
		Version: "1.2.3",
		Engine:  "docker",
		Backend: "claude",
		Image:   "ghcr.io/example/code-fixer:1.2.3",
	}

	tests := []struct {
		name         string
		ref          string
		notFoundName string // server returns 404 for this {name} segment
		wantPath     string
		wantName     string
		wantVersion  string
		wantErrIs    error
		wantMeta     *sandbox.SandboxBundleMetadata
	}{
		{
			// Scenario: Resolve a published definition.
			name:        "explicit name and version",
			ref:         "code-fixer@1.2.3",
			wantPath:    "/api/v1/agents/code-fixer",
			wantName:    "code-fixer",
			wantVersion: "1.2.3",
			wantMeta:    &wantMeta,
		},
		{
			// Scenario: Default to latest.
			name:        "no version defaults to latest",
			ref:         "code-fixer",
			wantPath:    "/api/v1/agents/code-fixer",
			wantName:    "code-fixer",
			wantVersion: "latest",
			wantMeta:    &wantMeta,
		},
		{
			// Edge: trailing @ with empty version defaults to latest.
			name:        "trailing at sign defaults to latest",
			ref:         "code-fixer@",
			wantPath:    "/api/v1/agents/code-fixer",
			wantName:    "code-fixer",
			wantVersion: "latest",
			wantMeta:    &wantMeta,
		},
		{
			// Scenario: Unknown reference is not found.
			name:         "unknown reference is not found",
			ref:          "ghost@9.9.9",
			notFoundName: "ghost",
			wantPath:     "/api/v1/agents/ghost",
			wantName:     "ghost",
			wantVersion:  "9.9.9",
			wantErrIs:    runner.ErrDefinitionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, rec := newCatalogServer(t, wantMeta, tt.notFoundName)

			res := &HTTPDefinitionResolver{BaseURL: srv.URL, HTTPClient: srv.Client()}

			got, err := res.ResolveDefinition(context.Background(), tt.ref)

			// Request shape assertions (recorded regardless of outcome).
			if rec.path != tt.wantPath {
				t.Errorf("request path = %q, want %q", rec.path, tt.wantPath)
			}
			if rec.name != tt.wantName {
				t.Errorf("request name = %q, want %q", rec.name, tt.wantName)
			}
			if rec.version != tt.wantVersion {
				t.Errorf("request version = %q, want %q", rec.version, tt.wantVersion)
			}

			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				if got != nil {
					t.Errorf("metadata = %+v, want nil on not-found", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolveDefinition: unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("ResolveDefinition returned nil metadata, want %+v", tt.wantMeta)
			}
			if got.Name != tt.wantMeta.Name ||
				got.Version != tt.wantMeta.Version ||
				got.Engine != tt.wantMeta.Engine ||
				got.Backend != tt.wantMeta.Backend ||
				got.Image != tt.wantMeta.Image {
				t.Errorf("metadata = %+v, want %+v", got, tt.wantMeta)
			}
		})
	}
}
