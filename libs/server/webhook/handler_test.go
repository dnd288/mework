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
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/bus"
	"mework/libs/server/platform/secret"
	"mework/libs/server/platform/store"
	"mework/libs/server/provider"
	melloprovider "mework/libs/server/provider/mello"
	"mework/libs/shared/providers/mello"
)

// spyBroker records Publish calls for test assertions.
type spyBroker struct {
	mu        sync.Mutex
	published []publishRecord
}

type publishRecord struct {
	topic bus.Topic
	msg   bus.Message
}

func (s *spyBroker) Publish(_ context.Context, topic bus.Topic, msg bus.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, publishRecord{topic, msg})
	return nil
}

func (s *spyBroker) Subscribe(_ context.Context, _ bus.Identity, _ bus.Filter, _ string) (bus.Subscription, error) {
	return nil, nil
}

func (s *spyBroker) Ack(_ context.Context, _ string) error {
	return nil
}

func (s *spyBroker) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.published)
}

func (s *spyBroker) topics() []bus.Topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make([]bus.Topic, len(s.published))
	for i, p := range s.published {
		topics[i] = p.topic
	}
	return topics
}

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

	// Create broker spy and handler
	spy := &spyBroker{}
	handler := NewHandler(pool, spy, secretKey, mockMello.URL)

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

	// Payload with valid @mework trigger
	validPayload := []byte(`{
		"id": "event-123",
		"type": "comment.added",
		"actor": { "id": "user-456", "name": "Alice" },
		"model": { "type": "ticket", "board_id": "board-789" },
		"data": { "id": "comment-abc", "body": "@mework dev review fix the bug", "ticket_id": "ticket-999" }
	}`)

	t.Run("valid webhook publishes to runner.dev.dispatch", func(t *testing.T) {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(validPayload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(validPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-1")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
		}

		// Assert that a message was published to the broker instead of
		// directly querying the jobs table for an enqueued row.
		if spy.callCount() != 1 {
			t.Fatalf("expected 1 publish via broker, got %d", spy.callCount())
		}
		expectedTopic := bus.FormatTopic(bus.TopicRunnerDispatch, "dev")
		if spy.topics()[0] != expectedTopic {
			t.Errorf("expected topic %s, got %s", expectedTopic, spy.topics()[0])
		}
	})

	t.Run("duplicate delivery id is idempotent", func(t *testing.T) {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(validPayload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(validPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-1") // Duplicate

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for duplicate event, got %d", rec.Code)
		}

		// Assert no additional publish was made (still exactly 1).
		if spy.callCount() != 1 {
			t.Errorf("expected publish count to remain 1 for duplicate delivery, got %d", spy.callCount())
		}
	})

	t.Run("distinct events publish distinct messages", func(t *testing.T) {
		distinctPayload := []byte(`{
			"id": "event-126",
			"type": "comment.added",
			"actor": { "id": "user-456", "name": "Alice" },
			"model": { "type": "ticket", "board_id": "board-789" },
			"data": { "id": "comment-jkl", "body": "@mework dev cook another task", "ticket_id": "ticket-999" }
		}`)
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(distinctPayload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(distinctPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-4")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d, body: %s", rec.Code, rec.Body.String())
		}

		// Assert a second publish was made (distinct event).
		if spy.callCount() != 2 {
			t.Errorf("expected 2 publishes for two distinct events, got %d", spy.callCount())
		}
	})

	t.Run("no trigger returns 200 without publish", func(t *testing.T) {
		noTriggerPayload := []byte(`{
			"id": "event-127",
			"type": "comment.added",
			"actor": { "id": "user-456", "name": "Alice" },
			"model": { "type": "ticket", "board_id": "board-789" },
			"data": { "id": "comment-mno", "body": "regular comment without trigger keyword", "ticket_id": "ticket-999" }
		}`)
		ts := fmt.Sprintf("%d", time.Now().Unix())
		sig := computeSig(noTriggerPayload, ts)

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(noTriggerPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", sig)
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-5")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for non-trigger comment, got %d", rec.Code)
		}

		// Assert no additional publish was made (still 2 from previous tests).
		if spy.callCount() != 2 {
			t.Errorf("expected publish count to remain 2 (no trigger), got %d", spy.callCount())
		}
	})

	t.Run("invalid signature returns 401", func(t *testing.T) {
		ts := fmt.Sprintf("%d", time.Now().Unix())

		req := httptest.NewRequest("POST", "/webhooks/mello", strings.NewReader(string(validPayload)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Mello-Signature", "sha256=invalid-signature-hash")
		req.Header.Set("X-Mello-Timestamp", ts)
		req.Header.Set("X-Mello-Delivery-Id", "delivery-uuid-6")

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid signature, got %d", rec.Code)
		}
	})

	t.Run("unknown provider returns 404", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/webhooks/unknownprovider", strings.NewReader(string(validPayload)))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404 for unknown provider, got %d", rec.Code)
		}
	})
}
