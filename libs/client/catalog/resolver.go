// Package catalog provides an HTTP-backed resolver that maps a prebuilt
// definition reference to its sandbox bundle metadata by querying the server
// catalog.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"mework/libs/client/runner"
	"mework/libs/sandbox"
)

// defaultVersion is the version requested when a reference carries no explicit
// version (e.g. "code-fixer" or "code-fixer@").
const defaultVersion = "latest"

// HTTPDefinitionResolver resolves a "name@version" reference against the server
// catalog over HTTP and decodes the published definition into sandbox bundle
// metadata.
type HTTPDefinitionResolver struct {
	// BaseURL is the server base URL (e.g. "https://host:8080"), without a
	// trailing /api/v1.
	BaseURL string
	// HTTPClient performs the request; when nil http.DefaultClient is used.
	HTTPClient *http.Client
}

// compile-time assertion that the resolver satisfies the runner contract.
var _ runner.DefinitionResolver = (*HTTPDefinitionResolver)(nil)

// agentVersion mirrors the server's catalog.AgentVersion JSON shape closely
// enough to recover the definition payload.
type agentVersion struct {
	Payload []byte `json:"payload,omitempty"`
}

// ResolveDefinition resolves ref against GET /api/v1/agents/{name}?version=<v>.
// A missing version defaults to "latest". A 404 maps to
// runner.ErrDefinitionNotFound.
func (r *HTTPDefinitionResolver) ResolveDefinition(ctx context.Context, ref string) (*sandbox.SandboxBundleMetadata, error) {
	name, version := splitRef(ref)

	u := fmt.Sprintf("%s/api/v1/agents/%s?version=%s",
		strings.TrimRight(r.BaseURL, "/"),
		url.PathEscape(name),
		url.QueryEscape(version),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build catalog request: %w", err)
	}

	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("resolve %q: %w", ref, runner.ErrDefinitionNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("resolve %q: catalog returned %d: %s", ref, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var ver agentVersion
	if err := json.NewDecoder(resp.Body).Decode(&ver); err != nil {
		return nil, fmt.Errorf("decode catalog response: %w", err)
	}

	var meta sandbox.SandboxBundleMetadata
	if err := json.Unmarshal(ver.Payload, &meta); err != nil {
		return nil, fmt.Errorf("decode definition payload: %w", err)
	}
	return &meta, nil
}

// splitRef splits a reference on the first "@" into name and version. An empty
// or absent version defaults to "latest".
func splitRef(ref string) (name, version string) {
	name = ref
	if i := strings.Index(ref, "@"); i >= 0 {
		name = ref[:i]
		version = ref[i+1:]
	}
	if version == "" {
		version = defaultVersion
	}
	return name, version
}
