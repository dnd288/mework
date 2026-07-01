package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeFile is a small DAMP helper: create parent dirs and write content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// zipEntryNames returns the sorted list of entry names in a zip archive.
func zipEntryNames(t *testing.T, zipBytes []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return names
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestPack_RoundTrip packs a workspace dir into a bundle, then extracts it into
// a fresh dir, asserting mework.yml + .claude/settings.json + a regular file all
// survive with identical contents (delta-spec "Pull recreates the workspace").
func TestPack_RoundTrip(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		"mework.yml":            "name: my-workspace\nbackend: local\nengine: claude\n",
		".claude/settings.json": `{"hooks":[]}`,
		"README.md":             "# my workspace\n",
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(src, rel), content)
	}

	bundle, err := Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if len(bundle) == 0 {
		t.Fatal("Pack returned empty bundle")
	}

	dst := t.TempDir()
	if err := ExtractWorkspace(bundle, dst); err != nil {
		t.Fatalf("ExtractWorkspace: %v", err)
	}

	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Fatalf("read extracted %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("extracted %s = %q, want %q", rel, string(got), want)
		}
	}
}

// TestPack_IncludesNestedClaude asserts the zip entry names preserve mework.yml
// at the root and the nested .claude/settings.json path.
func TestPack_IncludesNestedClaude(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "mework.yml"), "name: ws\nbackend: local\n")
	writeFile(t, filepath.Join(src, ".claude", "settings.json"), "{}")

	bundle, err := Pack(src)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	names := zipEntryNames(t, bundle)
	for _, want := range []string{"mework.yml", ".claude/settings.json"} {
		if !contains(names, want) {
			t.Errorf("zip entries %v missing %q", names, want)
		}
	}
}

// TestPack_MissingDirIsError asserts packing a non-existent dir surfaces an error
// rather than producing an empty/garbage bundle.
func TestPack_MissingDirIsError(t *testing.T) {
	if _, err := Pack(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error packing a missing directory, got nil")
	}
}

// TestPush_PostsBundleToVersionsEndpoint stands up an httptest server asserting
// Push POSTs the zip bundle to /api/v1/agents/{name}/versions tagged form=bundle,
// and that the payload decodes back to the exact bundle bytes (delta-spec
// "Pack then push").
func TestPush_PostsBundleToVersionsEndpoint(t *testing.T) {
	bundle := []byte("PK-fake-zip-bytes")

	var gotPath, gotMethod, gotForm, gotVersion string
	var payloadMatches bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		var req struct {
			Version string `json:"version"`
			Form    string `json:"form"`
			Payload string `json:"payload"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		gotForm = req.Form
		gotVersion = req.Version
		if decoded, err := base64.StdEncoding.DecodeString(req.Payload); err == nil {
			payloadMatches = bytes.Equal(decoded, bundle)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"version":"v1"}`))
	}))
	defer srv.Close()

	if err := Push(context.Background(), srv.URL, "my-workspace", "v1", bundle); err != nil {
		t.Fatalf("Push: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/agents/my-workspace/versions" {
		t.Errorf("path = %q, want /api/v1/agents/my-workspace/versions", gotPath)
	}
	if gotForm != "bundle" {
		t.Errorf("form = %q, want bundle", gotForm)
	}
	if gotVersion != "v1" {
		t.Errorf("version = %q, want v1", gotVersion)
	}
	if !payloadMatches {
		t.Error("posted payload did not decode back to the bundle bytes")
	}
}

// TestPush_RejectsServerError asserts a non-2xx response surfaces as an error
// (e.g. re-pushing an existing immutable version → 409).
func TestPush_RejectsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "version already exists", http.StatusConflict)
	}))
	defer srv.Close()

	if err := Push(context.Background(), srv.URL, "ws", "v1", []byte("x")); err == nil {
		t.Fatal("expected error on 409 conflict, got nil")
	}
}
