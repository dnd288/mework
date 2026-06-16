package webhook

import (
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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/internal/mello"
	"mework/internal/server/provider"
	melloprovider "mework/internal/server/provider/mello"
	"mework/internal/server/secret"
	"mework/internal/store"
)

func TestWebhookHandler(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration webhook handler test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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

	// Clear DB tables
	_, err = pool.Exec(ctx, "DELETE FROM jobs; DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM runtimes; DELETE FROM profiles; DELETE FROM provider_connections; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	secretKey := "mework_super_secret_aes_key_123"
	webhookSecret := "mello_webhook_signing_secret"
	melloToken := "test_mello_access_token"

	// 2. Insert test data
	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Webhook Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	encryptedToken, err := secret.Seal(melloToken, secretKey)
	if err != nil {
		t.Fatalf("failed to encrypt token: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO provider_connections (account_id, provider_code, webhook_secret, mcp_auth_enc)
		VALUES ($1, $2, $3, $4)
	`, accountID, "mello", webhookSecret, encryptedToken)
	if err != nil {
		t.Fatalf("failed to insert provider connection: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id)
		VALUES ($1, $2, $3)
	`, accountID, "mello", "user-456")
	if err != nil {
		t.Fatalf("failed to insert account identity: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO watched_containers (account_id, provider_code, external_container_id)
		VALUES ($1, $2, $3)
	`, accountID, "mello", "board-789")
	if err != nil {
		t.Fatalf("failed to insert watched container: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup)
		VALUES ($1, $2, $3, $4)
	`, accountID, "dev", "Dev Machine", "lookup-hash-rt1")
	if err != nil {
		t.Fatalf("failed to insert runtime: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO profiles (account_id, name, body)
		VALUES ($1, $2, $3)
	`, accountID, "dev", "system prompt content for dev profile")
	if err != nil {
		t.Fatalf("failed to insert profile: %v", err)
	}

	// 3. Setup mock Mello server for fetching ticket snapshots
	mockMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHeader := r.Header.Get("Authorization")
		if tokenHeader != "Bearer "+melloToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/tickets/ticket-999" {
			ticket := mello.TicketDetail{
				Ticket: mello.Ticket{
					ID:          "ticket-999",
					Title:       "Test Ticket Title",
					Description: "Test Ticket Description",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(ticket)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockMello.Close()

	// 4. Register Mello provider in the registry
	adapter := melloprovider.NewMelloAdapter(mockMello.URL)
	provider.Register(adapter)

	handler := NewHandler(pool, secretKey, mockMello.URL)

	// Router for dispatching path params
	r := chi.NewRouter()
	r.Post("/webhooks/{provider}", handler.ServeHTTP)

	// Helper to compute HMAC signature
	computeSig := func(body []byte, ts string) string {
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write([]byte(ts))
		mac.Write([]byte("."))
		mac.Write(body)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	// Payload struct
	payload := []byte(`{
		"id": "event-123",
		"type": "comment.added",
		"actor": { "id": "user-456", "name": "Alice" },
		"model": { "type": "ticket", "board_id": "board-789" },
		"data": { "id": "comment-abc", "body": "@mework dev review fix the bug", "ticket_id": "ticket-999" }
	}`)

	// Test case 1: Valid payload & signature & authorized actor -> 202 Accepted, job enqueued
	{
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(payload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-1")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
		}

		// Verify job is enqueued in database
		var instructions string
		var taskTitle, taskDesc string
		var profileSnapshot *string
		err = pool.QueryRow(ctx, `
			SELECT instructions, task_title, task_description, profile_body_snapshot
			FROM jobs
			WHERE external_event_id = 'delivery-uuid-1'
		`).Scan(&instructions, &taskTitle, &taskDesc, &profileSnapshot)

		if err != nil {
			t.Fatalf("failed to query jobs table: %v", err)
		}
		if instructions != "fix the bug" {
			t.Errorf("expected instructions 'fix the bug', got: %s", instructions)
		}
		if taskTitle != "Test Ticket Title" || taskDesc != "Test Ticket Description" {
			t.Errorf("unexpected snapshotted title/description: title=%q, desc=%q", taskTitle, taskDesc)
		}
		if profileSnapshot == nil || *profileSnapshot != "system prompt content for dev profile" {
			t.Errorf("unexpected profile snapshot: %v", profileSnapshot)
		}
	}

	// Test case 2: Duplicate delivery_id -> 200 OK (graceful no-op)
	{
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(payload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-1") // Duplicate

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for duplicate event, got %d", rec.Code)
		}

		var count int
		err = pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected total jobs count to remain 1, got %d", count)
		}
	}

	// Test case 3: Unmapped container -> 200 OK (silent ignore)
	{
		unmappedPayload := []byte(`{
			"id": "event-124",
			"type": "comment.added",
			"actor": { "id": "user-456", "name": "Alice" },
			"model": { "type": "ticket", "board_id": "unmapped-board" },
			"data": { "id": "comment-abc", "body": "@mework dev review fix the bug", "ticket_id": "ticket-999" }
		}`)
		ts := fmt.Sprintf("%d", time.Now().Unix())

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(unmappedPayload)))
		req.Header.Set("Content-Type", "application/json")
		// Signature is computed with the secret (but lookup will actually fail closed before signature verification)
		sig := computeSig(unmappedPayload, ts)
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-2")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for unmapped container, got %d", rec.Code)
		}
	}

	// Test case 4: Unauthorized actor -> 200 OK (silent ignore)
	{
		unauthPayload := []byte(`{
			"id": "event-125",
			"type": "comment.added",
			"actor": { "id": "unauthorized-user", "name": "Eve" },
			"model": { "type": "ticket", "board_id": "board-789" },
			"data": { "id": "comment-abc", "body": "@mework dev review fix the bug", "ticket_id": "ticket-999" }
		}`)
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(unauthPayload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(unauthPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-3")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for unauthorized actor, got %d", rec.Code)
		}
	}

	// Test case 5: Invalid signature -> 401 Unauthorized
	{
		ts := fmt.Sprintf("%d", time.Now().Unix())

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(payload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", "sha256=invalid-signature-hash")
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-4")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid signature, got %d", rec.Code)
		}
	}

	// Test case 6: Unknown provider -> 404 Not Found
	{
		req := httptest.NewRequest("POST", "/webhooks/unknownprovider", strings.NewReader(string(payload)))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 for unknown provider, got %d", rec.Code)
		}
	}
}
