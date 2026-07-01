package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mework/libs/server/hub"
	"mework/libs/server/platform/store"
	"mework/libs/server/platform/token"
)

// enqueueResponse is the JSON response body for the job enqueue endpoint.
type enqueueResponse struct {
	JobID string `json:"job_id"`
}

// TestJobsEnqueueEndpoint verifies the POST /api/v1/jobs/enqueue endpoint.
func TestJobsEnqueueEndpoint(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping job enqueue integration test")
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

	// Clean DB
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

	// Setup
	serverKey := "test-server-key-16chars"
	secretKey := "test-secret-key-16ch-x"

	// Seed account and runtime with a known token_lookup so the runtime
	// auth middleware can authenticate via Bearer token.
	var accountID string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Enqueue Test Account') RETURNING id").Scan(&accountID)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	rtToken := "rt_enqueue-test-token"
	tokenLookup := token.ComputeLookup(rtToken, serverKey)

	var runtimeID string
	err = pool.QueryRow(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup)
		VALUES ($1, 'enqueue-test', 'Enqueue Test Worker', $2)
		RETURNING id
	`, accountID, tokenLookup).Scan(&runtimeID)
	if err != nil {
		t.Fatalf("seed runtime: %v", err)
	}

	// Start the full hub server (the enqueue route is NOT registered yet in RED).
	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ListenAddr:      "127.0.0.1:0",
		ServerKey:       serverKey,
		MeworkSecretKey: secretKey,
	}
	srv := hub.NewServer(pool, cfg)
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	enqueueURL := httpSrv.URL + "/api/v1/jobs/enqueue"
	_ = runtimeID // runtimeID is used by the handler at GREEN; unused at RED

	// -----------------------------------------------------------------------
	// Subtest: valid enqueue creates queued job
	// -----------------------------------------------------------------------
	var firstJobID string
	t.Run("valid enqueue creates queued job", func(t *testing.T) {
		payload := map[string]string{
			"provider_code": "mezon",
			"channel_id":    "ch_abc",
			"sender_id":     "user-1",
			"text":          "hello",
			"message_id":    "msg-001",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", enqueueURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rtToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
		}

		var respBody enqueueResponse
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if respBody.JobID == "" {
			t.Fatal("expected non-empty job ID in response")
		}
		firstJobID = respBody.JobID

		// Verify DB state matches the mapping:
		//   channel_id  → external_task_id
		//   message_id  → external_event_id
		//   sender_id   → external_actor_id
		//   text        → instructions
		//   provider_code → provider_code
		var dbStatus, dbProvider, dbEventID, dbTaskID, dbActorID, dbInstr string
		err = pool.QueryRow(ctx, `
			SELECT status, provider_code, external_event_id, external_task_id, external_actor_id, instructions
			FROM jobs WHERE id = $1
		`, firstJobID).Scan(&dbStatus, &dbProvider, &dbEventID, &dbTaskID, &dbActorID, &dbInstr)
		if err != nil {
			t.Fatalf("query job from db: %v", err)
		}
		if dbStatus != "queued" {
			t.Errorf("status = %q, want %q", dbStatus, "queued")
		}
		if dbProvider != "mezon" {
			t.Errorf("provider_code = %q, want %q", dbProvider, "mezon")
		}
		if dbEventID != "msg-001" {
			t.Errorf("external_event_id = %q, want %q", dbEventID, "msg-001")
		}
		if dbTaskID != "ch_abc" {
			t.Errorf("external_task_id = %q, want %q", dbTaskID, "ch_abc")
		}
		if dbActorID != "user-1" {
			t.Errorf("external_actor_id = %q, want %q", dbActorID, "user-1")
		}
		if dbInstr != "hello" {
			t.Errorf("instructions = %q, want %q", dbInstr, "hello")
		}
	})

	// -----------------------------------------------------------------------
	// Subtest: duplicate message_id returns 200 with same job ID (dedup)
	// -----------------------------------------------------------------------
	t.Run("duplicate message_id returns 200 with same job ID", func(t *testing.T) {
		if firstJobID == "" {
			t.Skip("firstJobID not available; cannot test dedup")
		}

		payload := map[string]string{
			"provider_code": "mezon",
			"channel_id":    "ch_abc",
			"sender_id":     "user-1",
			"text":          "hello",
			"message_id":    "msg-001",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", enqueueURL, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+rtToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for duplicate, got %d", resp.StatusCode)
		}

		var respBody enqueueResponse
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if respBody.JobID != firstJobID {
			t.Errorf("expected same job ID %q, got %q", firstJobID, respBody.JobID)
		}
	})

	// -----------------------------------------------------------------------
	// Table-driven validation error cases
	// -----------------------------------------------------------------------
	validPayload := map[string]string{
		"provider_code": "mezon",
		"channel_id":    "ch_abc",
		"sender_id":     "user-1",
		"text":          "hello",
		"message_id":    "msg-001",
	}

	validationCases := []struct {
		name           string
		token          string
		payload        map[string]string
		expectedStatus int
	}{
		{
			name:           "missing provider_code returns 400",
			token:          rtToken,
			payload:        map[string]string{"channel_id": "ch_abc", "sender_id": "user-1", "text": "hello", "message_id": "m1"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing channel_id returns 400",
			token:          rtToken,
			payload:        map[string]string{"provider_code": "mezon", "sender_id": "user-1", "text": "hello", "message_id": "m1"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing text returns 400",
			token:          rtToken,
			payload:        map[string]string{"provider_code": "mezon", "channel_id": "ch_abc", "sender_id": "user-1", "message_id": "m1", "text": ""},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing message_id returns 400",
			token:          rtToken,
			payload:        map[string]string{"provider_code": "mezon", "channel_id": "ch_abc", "sender_id": "user-1", "text": "hello"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "oversized text returns 413",
			token:          rtToken,
			payload:        map[string]string{"provider_code": "mezon", "channel_id": "ch_abc", "sender_id": "user-1", "text": strings.Repeat("a", 65537), "message_id": "m-oversized"},
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "unauthenticated request returns 401",
			token:          "",
			payload:        validPayload,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			req, _ := http.NewRequest("POST", enqueueURL, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}
		})
	}
}
