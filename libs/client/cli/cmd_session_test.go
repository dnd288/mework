package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"mework/libs/server/session"
)

// sessionTestEnv points the session commands at a stub server with a PAT, using
// an isolated config home so no real ~/.mework state leaks in. It returns a
// cleanup that restores the prior environment.
func sessionTestEnv(t *testing.T, serverURL, pat string) {
	t.Helper()
	t.Setenv("MEWORK_HOME", t.TempDir())
	t.Setenv("MEWORK_SERVER_URL", serverURL)
	if pat == "" {
		t.Setenv("MELLO_API_KEY", "")
	} else {
		t.Setenv("MELLO_API_KEY", pat)
	}
}

// runSession executes a freshly-built leaf command with the given args and
// captures combined stdout/stderr.
func runSession(cmd *cobra.Command, args ...string) (string, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.RunE(cmd, args)
	return out.String(), err
}

func TestSessionList_Table(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/sessions" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer pat-xyz" {
			http.Error(w, "missing PAT", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{"ID":"sess-1","Runner":"runner-a","Agent":{"Name":"demo"},"Status":"active"},
			{"ID":"sess-2","Runner":"runner-b","Agent":{"Name":"other"},"Status":"idle"}
		]`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	out, err := runSession(sessionListCmd)
	if err != nil {
		t.Fatalf("session list: %v", err)
	}
	for _, want := range []string{"SESSION ID", "sess-1", "demo", "active", "sess-2", "other", "idle"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\n%s", want, out)
		}
	}
}

func TestSessionList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"ID":"sess-1","Agent":{"Name":"demo"},"Status":"active"}]`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	cmd := sessionListCmd
	_ = cmd.Flags().Set("json", "true")
	defer cmd.Flags().Set("json", "false")
	out, err := runSession(cmd)
	if err != nil {
		t.Fatalf("session list --json: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(decoded) != 1 || decoded[0]["ID"] != "sess-1" {
		t.Errorf("unexpected JSON: %s", out)
	}
}

func TestSessionList_NoPAT(t *testing.T) {
	sessionTestEnv(t, "http://127.0.0.1:0", "")
	_, err := runSession(sessionListCmd)
	if err == nil {
		t.Fatal("expected error when PAT is missing")
	}
	if !strings.Contains(err.Error(), "login") && !strings.Contains(err.Error(), "authenticated") {
		t.Errorf("error should guide the user to log in, got: %v", err)
	}
}

func TestSessionCreate(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ID":"sess-new","Agent":{"Name":"demo"},"Status":"active"}`)
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	cmd := sessionCreateCmd
	_ = cmd.Flags().Set("agent", "demo")
	_ = cmd.Flags().Set("runner", "runner-a")
	_ = cmd.Flags().Set("version", "v2")
	defer func() {
		_ = cmd.Flags().Set("agent", "")
		_ = cmd.Flags().Set("runner", "")
		_ = cmd.Flags().Set("version", "")
	}()

	out, err := runSession(cmd)
	if err != nil {
		t.Fatalf("session create: %v", err)
	}
	if !strings.Contains(out, "sess-new") {
		t.Errorf("output should print the new session id, got: %s", out)
	}
	if gotBody["agent_name"] != "demo" || gotBody["runner"] != "runner-a" || gotBody["version"] != "v2" {
		t.Errorf("unexpected request body: %#v", gotBody)
	}
}

func TestSessionCreate_RequiresAgent(t *testing.T) {
	sessionTestEnv(t, "http://127.0.0.1:0", "pat-xyz")
	cmd := sessionCreateCmd
	_ = cmd.Flags().Set("agent", "")
	_, err := runSession(cmd)
	if err == nil {
		t.Fatal("expected error when --agent is missing")
	}
}

func TestSessionSend(t *testing.T) {
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

	_, err := runSession(sessionSendCmd, "sess-1", "hello world")
	if err != nil {
		t.Fatalf("session send: %v", err)
	}
	if gotBody["role"] != "user" || gotBody["content"] != "hello world" {
		t.Errorf("unexpected message body: %#v", gotBody)
	}
}

func TestSessionClose(t *testing.T) {
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

	_, err := runSession(sessionCloseCmd, "sess-1")
	if err != nil {
		t.Fatalf("session close: %v", err)
	}
	if !hit {
		t.Error("expected DELETE /api/v1/sessions/sess-1 to be called")
	}
}

func TestSessionAttach_StreamsAndStopsOnDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions/sess-1/stream" {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(ev session.ChatEvent) {
			data, _ := json.Marshal(ev)
			_, _ = io.WriteString(w, "data: "+string(data)+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
		write(session.ChatEvent{Kind: session.EventToken, Content: "Hel"})
		write(session.ChatEvent{Kind: session.EventMessage, Content: "Hello"})
		write(session.ChatEvent{Kind: session.EventDone})
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	out, err := runSession(sessionAttachCmd, "sess-1")
	if err != nil {
		t.Fatalf("session attach: %v", err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("attach should print streamed content, got: %s", out)
	}
}

func TestSessionAttach_IdleTimeout(t *testing.T) {
	// Server holds the connection open and never sends a terminal event.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	sessionTestEnv(t, srv.URL, "pat-xyz")

	cmd := sessionAttachCmd
	_ = cmd.Flags().Set("idle", "150ms")
	defer cmd.Flags().Set("idle", "")

	done := make(chan error, 1)
	go func() {
		_, err := runSession(cmd, "sess-1")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("attach should exit cleanly on idle, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attach did not exit on idle timeout")
	}
}
