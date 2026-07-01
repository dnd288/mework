package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mework/libs/shared/config"
)

// writeWorkspace creates a temp dir holding a mework.yml with the given name and
// version and returns the dir's absolute path.
func writeWorkspace(t *testing.T, name, version string) string {
	t.Helper()
	dir := t.TempDir()
	body := "name: " + name + "\nversion: " + version + "\n"
	if err := os.WriteFile(filepath.Join(dir, "mework.yml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write mework.yml: %v", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

// seedIdentity persists a runner identity under the test MEWORK_HOME so
// sandbox start can resolve the local runner. Call after sessionTestEnv.
func seedIdentity(t *testing.T, runnerID string) {
	t.Helper()
	if err := config.SaveIdentity(runnerID, "secret-xyz"); err != nil {
		t.Fatalf("save identity: %v", err)
	}
}

func TestSandboxStart_PostsWorkspaceBoundSession(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions" {
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer pat-xyz" {
			http.Error(w, "missing PAT", http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ID":"sess-ws","Agent":{"Name":"demo"},"Status":"active"}`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")
	seedIdentity(t, "runner-local")

	ws := writeWorkspace(t, "demo", "v1")

	cmd := sandboxStartCmd
	_ = cmd.Flags().Set("workspace", ws)
	defer cmd.Flags().Set("workspace", "")

	out, err := runSession(cmd)
	if err != nil {
		t.Fatalf("sandbox start: %v", err)
	}
	if !strings.Contains(out, "sess-ws") {
		t.Errorf("output should print the session id, got: %s", out)
	}
	if gotBody["agent_name"] != "demo" || gotBody["version"] != "v1" {
		t.Errorf("unexpected agent fields in body: %#v", gotBody)
	}
	if gotBody["runner"] != "runner-local" {
		t.Errorf("body should target the local runner, got: %#v", gotBody)
	}
	if gotBody["workspace"] != ws {
		t.Errorf("body should carry the absolute workspace path %q, got: %#v", ws, gotBody)
	}
}

func TestSandboxStart_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ID":"sess-ws","Agent":{"Name":"demo"},"Status":"active"}`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")
	seedIdentity(t, "runner-local")

	ws := writeWorkspace(t, "demo", "v1")

	cmd := sandboxStartCmd
	_ = cmd.Flags().Set("workspace", ws)
	_ = cmd.Flags().Set("json", "true")
	defer func() {
		_ = cmd.Flags().Set("workspace", "")
		_ = cmd.Flags().Set("json", "false")
	}()

	out, err := runSession(cmd)
	if err != nil {
		t.Fatalf("sandbox start --json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["ID"] != "sess-ws" {
		t.Errorf("unexpected JSON: %s", out)
	}
}

func TestSandboxStart_MissingWorkspaceConfig(t *testing.T) {
	sessionTestEnv(t, "http://127.0.0.1:0", "pat-xyz")
	seedIdentity(t, "runner-local")

	// A bare temp dir without mework.yml.
	dir := t.TempDir()
	cmd := sandboxStartCmd
	_ = cmd.Flags().Set("workspace", dir)
	defer cmd.Flags().Set("workspace", "")

	_, err := runSession(cmd)
	if err == nil {
		t.Fatal("expected error when mework.yml is missing")
	}
}

func TestSandboxStart_NotEnrolled(t *testing.T) {
	sessionTestEnv(t, "http://127.0.0.1:0", "pat-xyz")
	// No identity seeded → not enrolled.

	ws := writeWorkspace(t, "demo", "v1")
	cmd := sandboxStartCmd
	_ = cmd.Flags().Set("workspace", ws)
	defer cmd.Flags().Set("workspace", "")

	_, err := runSession(cmd)
	if err == nil {
		t.Fatal("expected error when the machine is not enrolled")
	}
	if !strings.Contains(err.Error(), "enroll") && !strings.Contains(err.Error(), "daemon") {
		t.Errorf("error should guide the user to enroll/start the daemon, got: %v", err)
	}
}

func TestSandboxList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/sessions" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"ID":"sess-1","Runner":"runner-a","Agent":{"Name":"demo"},"Status":"active"}]`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	out, err := runSession(sandboxListCmd)
	if err != nil {
		t.Fatalf("sandbox list: %v", err)
	}
	for _, want := range []string{"SESSION ID", "sess-1", "demo", "active"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\n%s", want, out)
		}
	}
}

func TestSandboxStop(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/sessions/sess-1" {
			hit = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	_, err := runSession(sandboxStopCmd, "sess-1")
	if err != nil {
		t.Fatalf("sandbox stop: %v", err)
	}
	if !hit {
		t.Error("expected DELETE /api/v1/sessions/sess-1 to be called")
	}
}

func TestSandboxSend(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions/sess-1/messages" {
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	_, err := runSession(sandboxSendCmd, "sess-1", "summarize the repo")
	if err != nil {
		t.Fatalf("sandbox send: %v", err)
	}
	if gotBody["role"] != "user" || gotBody["content"] != "summarize the repo" {
		t.Errorf("unexpected message body: %#v", gotBody)
	}
}

func TestSandboxCommandSurface(t *testing.T) {
	names := map[string]bool{}
	for _, c := range sandboxCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"start", "list", "stop", "send"} {
		if !names[want] {
			t.Errorf("sandbox group missing subcommand %q", want)
		}
	}
	if sandboxCmd.GroupID != groupRuntime {
		t.Errorf("sandbox group should be under groupRuntime, got %q", sandboxCmd.GroupID)
	}
}
