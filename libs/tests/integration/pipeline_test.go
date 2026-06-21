package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	meworkclient "mework/libs/client/subscribe"
	"mework/libs/server/hub"
	"mework/libs/server/platform/store"
	melloprovider "mework/libs/shared/providers/mello"
)

// pipelineCase defines one end-to-end scenario for the behavior-preservation
// test suite. Each case exercises a distinct delta-spec scenario from
// project-structure/spec.md.
type pipelineCase struct {
	name           string
	comment        string
	deliveryID     string
	expectJob      bool
	expectedStatus string // "queued" | "none"
	expectWrite    bool
}

// TestFullPipelineE2E_BehaviorPreservation validates that the webhook→enqueue→
// claim→ack→writeback pipeline works end-to-end after the repo restructure.
// This is the primary "behavior-preserving migration" guard (delta-spec:
// "Binaries and behavior unchanged").
func TestFullPipelineE2E_BehaviorPreservation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping E2E pipeline integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := store.RunMigrations(dsn); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	defer func() { _ = store.RollbackMigrations(dsn) }()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test db: %v", err)
	}
	defer pool.Close()

	// Clear DB
	if _, err := pool.Exec(ctx,
		`DELETE FROM jobs;
		 DELETE FROM watched_containers;
		 DELETE FROM account_identities;
		 DELETE FROM runtimes;
		 DELETE FROM profiles;
		 DELETE FROM provider_connections;
		 DELETE FROM accounts;`,
	); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	serverKey := "test-server-key"
	secretKey := "test-secret-key"
	webhookSecret := "test-webhook-secret"
	melloToken := "test-mello-pat"
	patToken := "user-pat-token"

	cases := []pipelineCase{
		{
			name:           "full flow: webhook→enqueue→claim→ack→writeback (behavior preservation)",
			comment:        "@mework dev review fix the bug",
			deliveryID:     "delivery-1",
			expectJob:      true,
			expectedStatus: "queued",
			expectWrite:    true,
		},
		{
			name:           "self-retrigger guard: own comment is skipped",
			comment:        "@mework dev review skip me",
			deliveryID:     "delivery-self",
			expectJob:      false,
			expectedStatus: "none",
			expectWrite:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock Mello — fresh counters per case
			meCallCount := 0
			ticketCallCount := 0
			writebackCallCount := 0
			var lastCommentBody string

			mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokenHeader := r.Header.Get("Authorization")

				if r.URL.Path == "/me" {
					if tokenHeader != "Bearer "+patToken {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					meCallCount++
					_ = json.NewEncoder(w).Encode(melloprovider.User{
						ID:    "mello-user-123",
						Email: "test@example.com",
						Name:  "Test User",
					})
					return
				}

				if tokenHeader != "Bearer "+melloToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				if r.Method == "GET" && r.URL.Path == "/tickets/tkt-999" {
					ticketCallCount++
					_ = json.NewEncoder(w).Encode(melloprovider.TicketDetail{
						Ticket: melloprovider.Ticket{
							ID:          "tkt-999",
							Title:       "Integration Test Ticket",
							Description: "This is a test ticket description",
						},
					})
					return
				}

				if r.Method == "POST" && r.URL.Path == "/tickets/tkt-999/comments" {
					writebackCallCount++
					var body struct {
						Body string `json:"body"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					lastCommentBody = body.Body
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":"comment-123"}`))
					return
				}

				w.WriteHeader(http.StatusNotFound)
			}))
			defer mockMello.Close()

			// Start mework-server
			cfg := &hub.Config{
				DatabaseURL:     dsn,
				ListenAddr:      "127.0.0.1:0",
				WebhookSecret:   webhookSecret,
				ServerKey:       serverKey,
				MeworkSecretKey: secretKey,
				MelloBaseURL:    mockMello.URL,
			}
			srv := hub.NewServer(pool, cfg)
			mockServer := httptest.NewServer(srv)
			defer mockServer.Close()

			client := meworkclient.NewClient(mockServer.URL, 5*time.Second)

			// Setup Connection, Runtime & Profile
			runtimeRes, err := client.CreateRuntime(patToken, "dev", "Dev Machine")
			if err != nil {
				t.Fatalf("CreateRuntime: %v", err)
			}
			if runtimeRes.Token == "" {
				t.Fatal("expected non-empty runtime token")
			}

			if _, err := client.CreateConnection(patToken, "mello", melloToken, webhookSecret, nil); err != nil {
				t.Fatalf("CreateConnection: %v", err)
			}

			if _, err := client.CreateProfile(patToken, meworkclient.CreateProfileRequest{
				Name:        "dev",
				Body:        "my system prompt",
				BackendHint: "claude",
				Harness:     "ck",
			}); err != nil {
				t.Fatalf("CreateProfile: %v", err)
			}

			// Seed watched container for the board
			if _, err := pool.Exec(ctx, `
				INSERT INTO watched_containers (account_id, provider_code, external_container_id)
				VALUES ($1, 'mello', 'board-789')
				ON CONFLICT DO NOTHING
			`, runtimeRes.AccountID); err != nil {
				t.Fatalf("seed watched container: %v", err)
			}

			// Simulate inbound webhook event
			payload := []byte(fmt.Sprintf(`{
				"id": "evt-uuid-1",
				"type": "comment.added",
				"actor": {"id": "mello-user-123", "name": "Test User"},
				"model": {"type": "ticket", "board_id": "board-789"},
				"data": {"id": "comment-uuid-1", "body": %q, "ticket_id": "tkt-999"}
			}`, tc.comment))

			ts := fmt.Sprintf("%d", time.Now().Unix())
			mac := hmac.New(sha256.New, []byte(webhookSecret))
			mac.Write([]byte(ts))
			mac.Write([]byte("."))
			mac.Write(payload)
			sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

			webhookURL := mockServer.URL + "/webhooks/mello"
			req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Mello-Signature", sig)
			req.Header.Set("X-Mello-Timestamp", ts)
			req.Header.Set("X-Mello-Delivery-Id", tc.deliveryID)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("webhook request: %v", err)
			}
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("expected 202 Accepted, got: %d", resp.StatusCode)
			}
			resp.Body.Close()

			// Wait for async webhook processing
			time.Sleep(200 * time.Millisecond)

			// Verify job enqueued (or not)
			if tc.expectJob {
				var jobID, status string
				err = pool.QueryRow(ctx, "SELECT id, status FROM jobs WHERE external_event_id = $1", tc.deliveryID).Scan(&jobID, &status)
				if err != nil {
					t.Fatalf("query job: %v", err)
				}
				if status != tc.expectedStatus {
					t.Errorf("expected status %q, got: %s", tc.expectedStatus, status)
				}

				// Claim → Ack running → Ack done → verify writeback
				job, err := client.Claim(runtimeRes.Token)
				if err != nil {
					t.Fatalf("Claim: %v", err)
				}
				if job == nil || job.ID != jobID {
					t.Fatalf("expected job %s to be claimed, got: %+v", jobID, job)
				}

				if err := client.Ack(runtimeRes.Token, jobID, "running", "", ""); err != nil {
					t.Fatalf("Ack running: %v", err)
				}

				if err := client.Ack(runtimeRes.Token, jobID, "done", "fixed the bug in auth middleware", ""); err != nil {
					t.Fatalf("Ack done: %v", err)
				}

				// Wait for async write-back
				time.Sleep(200 * time.Millisecond)

				if tc.expectWrite {
					if writebackCallCount != 1 {
						t.Errorf("expected 1 write-back, got %d", writebackCallCount)
					}
					if !strings.Contains(lastCommentBody, "mework dev review — done") ||
						!strings.Contains(lastCommentBody, "fixed the bug in auth middleware") {
						t.Errorf("unexpected comment body: %q", lastCommentBody)
					}
				}

				// Verify writeback status
				var wbStatus string
				if err := pool.QueryRow(ctx, "SELECT writeback_status FROM jobs WHERE id = $1", jobID).Scan(&wbStatus); err != nil {
					t.Fatalf("query writeback status: %v", err)
				}
				if wbStatus != "success" {
					t.Errorf("expected writeback_status 'success', got: %s", wbStatus)
				}
			} else {
				// No job expected — verify no row was created
				var count int
				if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM jobs WHERE external_event_id = $1", tc.deliveryID).Scan(&count); err != nil {
					t.Fatalf("count jobs: %v", err)
				}
				if count != 0 {
					t.Errorf("expected 0 jobs for delivery %q, got %d", tc.deliveryID, count)
				}
			}
		})
	}
}
