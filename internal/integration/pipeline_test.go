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

	"mework/internal/mello"
	"mework/internal/meworkclient"
	"mework/internal/server"
	"mework/internal/store"
)

func TestFullPipelineE2E(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping E2E pipeline integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Run migrations
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

	// Clear DB
	_, err = pool.Exec(ctx, "DELETE FROM jobs; DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM profiles; DELETE FROM provider_connections; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	serverKey := "test-server-key"
	secretKey := "test-secret-key"
	webhookSecret := "test-webhook-secret"
	melloToken := "test-mello-pat"
	patToken := "user-pat-token"

	// 2. Setup mock Mello server for both:
	// - GetCurrentUser (/me) during PAT auth
	// - GetTicket (/tickets/{id}) during Webhook handler
	// - CreateComment (/tickets/{id}/comments) during write-back execution
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
			user := mello.User{
				ID:    "mello-user-123",
				Email: "test@example.com",
				Name:  "Test User",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(user)
			return
		}

		if tokenHeader != "Bearer "+melloToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == "GET" && r.URL.Path == "/tickets/tkt-999" {
			ticketCallCount++
			ticket := mello.TicketDetail{
				Ticket: mello.Ticket{
					ID:          "tkt-999",
					Title:       "Integration Test Ticket",
					Description: "This is a test ticket description",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ticket)
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

	// 3. Start mework-server
	cfg := &server.Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0", // random port
		WebhookSecret:   webhookSecret,
		ServerKey:       serverKey,
		MeworkSecretKey: secretKey,
		MelloBaseURL:    mockMello.URL,
	}

	srv := server.NewServer(pool, cfg)
	mockServer := httptest.NewServer(srv)
	defer mockServer.Close()

	client := meworkclient.NewClient(mockServer.URL, 5*time.Second)

	// Step A: Setup Connection & Runtime & Profile
	// Since connecting/registering/profile CRUD requires PAT token, we first make PAT-authenticated calls
	// which will trigger the PATAuth middleware and upsert the account details in the database!

	// Register runtime first to trigger PAT resolution + account creation
	runtimeRes, err := client.CreateRuntime(patToken, "dev", "Dev Machine")
	if err != nil {
		t.Fatalf("failed to register runtime: %v", err)
	}
	if runtimeRes.Token == "" {
		t.Fatal("expected non-empty runtime token")
	}

	// Create provider connection
	_, err = client.CreateConnection(patToken, "mello", melloToken, webhookSecret, nil)
	if err != nil {
		t.Fatalf("failed to connect provider: %v", err)
	}

	// Create profile
	_, err = client.CreateProfile(patToken, meworkclient.CreateProfileRequest{
		Name:        "dev",
		Body:        "my system prompt",
		BackendHint: "claude",
		Harness:     "ck",
	})
	if err != nil {
		t.Fatalf("failed to create profile: %v", err)
	}

	// Make sure watched container is set for our board
	_, err = pool.Exec(ctx, `
		INSERT INTO watched_containers (account_id, provider_code, external_container_id)
		VALUES ($1, 'mello', 'board-789')
		ON CONFLICT DO NOTHING
	`, runtimeRes.AccountID)
	if err != nil {
		t.Fatalf("failed to seed watched container: %v", err)
	}

	// Step B: Simulate Inbound Webhook Event
	payload := []byte(`{
		"id": "evt-uuid-1",
		"type": "comment.added",
		"actor": { "id": "mello-user-123", "name": "Test User" },
		"model": { "type": "ticket", "board_id": "board-789" },
		"data": { "id": "comment-uuid-1", "body": "@mework dev review fix the bug", "ticket_id": "tkt-999" }
	}`)

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
	req.Header.Set("X-Mello-Delivery-Id", "delivery-1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook request failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Wait briefly for webhook async processing (GetTicket snapshot) to complete
	time.Sleep(200 * time.Millisecond)

	// Verify job enqueued
	var jobID, status string
	err = pool.QueryRow(ctx, "SELECT id, status FROM jobs WHERE external_event_id = 'delivery-1'").Scan(&jobID, &status)
	if err != nil {
		t.Fatalf("failed to query job: %v", err)
	}
	if status != "queued" {
		t.Errorf("expected status queued, got: %s", status)
	}

	// Step C: Daemon claims job
	job, err := client.Claim(runtimeRes.Token)
	if err != nil {
		t.Fatalf("failed to claim job: %v", err)
	}
	if job == nil || job.ID != jobID {
		t.Fatalf("expected job %s to be claimed, got: %+v", jobID, job)
	}

	// Step D: Ack running
	err = client.Ack(runtimeRes.Token, jobID, "running", "", "")
	if err != nil {
		t.Fatalf("failed to ack running: %v", err)
	}

	// Step E: Ack done (triggers write-back)
	err = client.Ack(runtimeRes.Token, jobID, "done", "fixed the bug in auth middleware", "")
	if err != nil {
		t.Fatalf("failed to ack done: %v", err)
	}

	// Wait briefly for async write-back to execute
	time.Sleep(200 * time.Millisecond)

	// Verify write-back happened on mock Mello server
	if writebackCallCount != 1 {
		t.Errorf("expected 1 write-back comment creation call, got %d", writebackCallCount)
	}
	if !strings.Contains(lastCommentBody, "mework dev review — done") || !strings.Contains(lastCommentBody, "fixed the bug in auth middleware") {
		t.Errorf("unexpected comment body in write-back: %q", lastCommentBody)
	}

	// Verify write-back status is success in database
	var writebackStatus string
	err = pool.QueryRow(ctx, "SELECT writeback_status FROM jobs WHERE id = $1", jobID).Scan(&writebackStatus)
	if err != nil {
		t.Fatalf("failed to query job writeback status: %v", err)
	}
	if writebackStatus != "success" {
		t.Errorf("expected writeback_status success, got: %s", writebackStatus)
	}
}
