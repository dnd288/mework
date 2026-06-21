package writeback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/platform/secret"
	"mework/libs/server/platform/store"
)

func TestFormatComment(t *testing.T) {
	tests := []struct {
		name         string
		profile      string
		workflow     string
		status       string
		summary      string
		lastError    string
		expectedBody string
	}{
		{
			name:         "done with summary and workflow",
			profile:      "dev",
			workflow:     "review",
			status:       "done",
			summary:      "fixed bug in auth",
			lastError:    "",
			expectedBody: "mework dev review — done\nfixed bug in auth",
		},
		{
			name:         "done with summary, no workflow",
			profile:      "dev",
			workflow:     "",
			status:       "done",
			summary:      "fixed bug in auth",
			lastError:    "",
			expectedBody: "mework dev — done\nfixed bug in auth",
		},
		{
			name:         "done with empty summary",
			profile:      "dev",
			workflow:     "",
			status:       "done",
			summary:      "",
			lastError:    "",
			expectedBody: "mework dev — done\ncompleted, no output",
		},
		{
			name:         "failed with error",
			profile:      "prod",
			workflow:     "ship",
			status:       "failed",
			summary:      "",
			lastError:    "timeout connection error",
			expectedBody: "mework prod ship — failed\ntimeout connection error",
		},
		{
			name:         "failed with summary and error",
			profile:      "prod",
			workflow:     "ship",
			status:       "failed",
			summary:      "partial success",
			lastError:    "failed at stage 3",
			expectedBody: "mework prod ship — failed\npartial success",
		},
		{
			name:         "failed with no message",
			profile:      "prod",
			workflow:     "",
			status:       "failed",
			summary:      "",
			lastError:    "",
			expectedBody: "mework prod — failed\nfailed without error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatComment(tt.profile, tt.workflow, tt.status, tt.summary, tt.lastError)
			if got != tt.expectedBody {
				t.Errorf("formatComment() = %q, want %q", got, tt.expectedBody)
			}
		})
	}
}

func TestFormatCommentTruncation(t *testing.T) {
	summary := strings.Repeat("A", 70000)
	got := formatComment("dev", "", "done", summary, "")
	if len(got) > 61500 {
		t.Errorf("expected output to be truncated to ~60KB, got len: %d", len(got))
	}
	if !strings.HasSuffix(got, "\n... [truncated]") {
		t.Error("expected output to end with truncation notice")
	}
}

func TestExecuteWriteBack(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := store.RunMigrations(dsn)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	defer func() {
		_ = store.RollbackMigrations(dsn)
	}()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, "DELETE FROM jobs; DELETE FROM runtimes; DELETE FROM provider_connections; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	secretKey := "mework_writeback_secret_key"
	melloToken := "pat_token_for_writeback"

	// Setup account & connection
	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	encryptedToken, err := secret.Seal(melloToken, secretKey)
	if err != nil {
		t.Fatalf("failed to encrypt token: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO provider_connections (account_id, provider_code, webhook_secret, mcp_auth_enc)
		VALUES ($1, $2, 'secret', $3)
	`, accountID, "mello", encryptedToken)
	if err != nil {
		t.Fatalf("failed to insert connection: %v", err)
	}

	var runtimeID string
	err = pool.QueryRow(ctx, "INSERT INTO runtimes (account_id, code, label, token_lookup) VALUES ($1, 'dev', 'Dev Machine', 'lookup-hash') RETURNING id", accountID).Scan(&runtimeID)
	if err != nil {
		t.Fatalf("failed to insert runtime: %v", err)
	}

	// Insert done job
	var jobID string
	err = pool.QueryRow(ctx, `
		INSERT INTO jobs (
			account_id, runtime_id, external_task_id, external_event_id, provider_code,
			task_title, task_description, instructions, status, ttl_expires_at,
			result_summary
		) VALUES ($1, $2, 'tkt-123', 'evt-123', 'mello', 'Title', 'Desc', 'inst', 'done', NOW() + INTERVAL '1 hour', 'job completed successfully')
		RETURNING id
	`, accountID, runtimeID).Scan(&jobID)
	if err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}

	// Setup mock Mello server for write-back comment creation
	writebackReceived := false
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHeader := r.Header.Get("Authorization")
		if tokenHeader != "Bearer "+melloToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == "POST" && r.URL.Path == "/tickets/tkt-123/comments" {
			var body struct {
				Body string `json:"body"`
			}
			err := json.NewDecoder(r.Body).Decode(&body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if !strings.Contains(body.Body, "mework dev — done") || !strings.Contains(body.Body, "job completed successfully") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			writebackReceived = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"comment-123","body":"..."}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockMello.Close()

	// Execute writeback
	err = ExecuteWriteBack(ctx, pool, secretKey, mockMello.URL, jobID)
	if err != nil {
		t.Fatalf("ExecuteWriteBack failed: %v", err)
	}

	if !writebackReceived {
		t.Error("expected mock Mello server to receive write-back comment, but it was not called")
	}
}
