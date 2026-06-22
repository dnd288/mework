package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// executeEnroll runs runnerEnrollCmd with the given args, capturing stdout, and
// returns the captured output and any RunE error. A context is supplied because
// RunE uses cmd.Context() (which cobra populates only via Execute()).
func executeEnroll(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	runnerEnrollCmd.SetOut(&out)
	runnerEnrollCmd.SetErr(&out)
	runnerEnrollCmd.SetArgs(args)
	runnerEnrollCmd.SetContext(context.Background())
	err := runnerEnrollCmd.RunE(runnerEnrollCmd, args)
	return out.String(), err
}

// TestRunnerEnroll exercises the real enrollment handshake: a 200 persists the
// identity and prints the runner ID; a 4xx surfaces an error and persists
// nothing; missing required flags fail before any network call.
func TestRunnerEnroll(t *testing.T) {
	tests := []struct {
		name string
		// status/body describe the stub hub response; status==0 means "no server"
		status     int
		body       string
		omitURL    bool
		omitToken  bool
		token      string
		wantErr    bool
		wantStdout string // substring expected in stdout on success
		wantSaved  bool   // identity file should exist afterward
	}{
		{
			name:       "success persists identity and prints runner id",
			status:     http.StatusOK,
			body:       `{"runner_id":"runner-abc123","secret":"s3cr3t"}`,
			token:      "good-token",
			wantErr:    false,
			wantStdout: "runner-abc123",
			wantSaved:  true,
		},
		{
			name:      "hub rejection returns error and saves nothing",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid token"}`,
			token:     "bad-token",
			wantErr:   true,
			wantSaved: false,
		},
		{
			name:      "missing url fails before network call",
			omitURL:   true,
			token:     "good-token",
			wantErr:   true,
			wantSaved: false,
		},
		{
			name:      "missing token fails before network call",
			omitToken: true,
			wantErr:   true,
			wantSaved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sandbox the identity path so the real ~/.mework is untouched.
			home := t.TempDir()
			t.Setenv("MEWORK_HOME", home)
			identityFile := filepath.Join(home, "identity.json")

			var serverURL string
			if tt.status != 0 || !tt.omitURL {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runners/enroll" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					status := tt.status
					if status == 0 {
						status = http.StatusOK
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte(tt.body))
				}))
				defer srv.Close()
				serverURL = srv.URL
			}

			var args []string
			if !tt.omitURL {
				args = append(args, "--url", serverURL)
			}
			if !tt.omitToken {
				args = append(args, "--token", tt.token)
			}

			stdout, err := executeEnroll(t, args...)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil (stdout=%q)", stdout)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout %q does not contain %q", stdout, tt.wantStdout)
			}

			_, statErr := os.Stat(identityFile)
			saved := statErr == nil
			if saved != tt.wantSaved {
				t.Errorf("identity saved = %v, want %v", saved, tt.wantSaved)
			}
		})
	}
}
