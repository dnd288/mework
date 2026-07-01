package enroll

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"mework/libs/shared/config"
)

func TestEnrollClient(t *testing.T) {
	validHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runners/enroll" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"runner_id": "r-a1b2c3",
			"secret":    "sk-secret-abc123",
		})
	}))
	defer validHub.Close()

	invalidHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer invalidHub.Close()

	errorHub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorHub.Close()

	tests := []struct {
		name         string
		hubURL       string
		regToken     string
		wantErr      bool
		wantRunnerID string
		wantSecret   string
	}{
		{
			name:         "successful enrollment creates identity and persists to disk",
			hubURL:       validHub.URL,
			regToken:     "valid-reg-token",
			wantErr:      false,
			wantRunnerID: "r-a1b2c3",
			wantSecret:   "sk-secret-abc123",
		},
		{
			name:     "invalid token returns error and does not persist",
			hubURL:   invalidHub.URL,
			regToken: "invalid-token",
			wantErr:  true,
		},
		{
			name:     "server error returns error and does not persist",
			hubURL:   errorHub.URL,
			regToken: "reg-token",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("MEWORK_HOME", tmpDir)

			identity, err := Enroll(context.Background(), tt.hubURL, tt.regToken)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if _, statErr := os.Stat(config.IdentityPath()); statErr == nil {
					t.Error("identity file exists after failed enrollment; should not")
				} else if !os.IsNotExist(statErr) {
					t.Fatalf("stat identity file: %v", statErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Enroll() returned unexpected error: %v", err)
			}

			if identity.RunnerID != tt.wantRunnerID {
				t.Errorf("RunnerID = %q, want %q", identity.RunnerID, tt.wantRunnerID)
			}
			if identity.Secret != tt.wantSecret {
				t.Errorf("Secret = %q, want %q", identity.Secret, tt.wantSecret)
			}

			identityPath := config.IdentityPath()

			data, readErr := os.ReadFile(identityPath)
			if readErr != nil {
				t.Fatalf("read identity file: %v", readErr)
			}

			var persisted struct {
				RunnerID string `json:"runner_id"`
				Secret   string `json:"secret"`
			}
			if err := json.Unmarshal(data, &persisted); err != nil {
				t.Fatalf("unmarshal identity file: %v", err)
			}
			if persisted.RunnerID != tt.wantRunnerID {
				t.Errorf("persisted RunnerID = %q, want %q", persisted.RunnerID, tt.wantRunnerID)
			}
			if persisted.Secret != tt.wantSecret {
				t.Errorf("persisted Secret = %q, want %q", persisted.Secret, tt.wantSecret)
			}
		})
	}
}

func TestEnrollClient_PersistencePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MEWORK_HOME", tmpDir)

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"runner_id": "r-persist",
			"secret":    "sk-persist-test",
		})
	}))
	defer hub.Close()

	_, err := Enroll(context.Background(), hub.URL, "reg-token")
	if err != nil {
		t.Fatalf("Enroll() returned error: %v", err)
	}

	identityPath := config.IdentityPath()
	info, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("stat identity file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("identity file permissions = %o, want 600", info.Mode().Perm())
	}
}
