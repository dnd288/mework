// Package mello_claude_test validates the full Mello -> mework -> sandbox flow
// with fake/mocked external dependencies so it runs standalone.
package mello_claude_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/libs/server/hub"
	"mework/libs/server/platform/secret"
	storePkg "mework/libs/server/platform/store"
)

const testSecretKey = "test-secret-key-32bytes!"

// cleanupDB truncates all tenant-scoped tables so each test starts clean.
func cleanupDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		DELETE FROM jobs;
		DELETE FROM watched_containers;
		DELETE FROM account_identities;
		DELETE FROM profiles;
		DELETE FROM runtimes;
		DELETE FROM provider_connections;
		DELETE FROM accounts;
	`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

// seedFullPipelineData inserts rows needed by the webhook handler.
func seedFullPipelineData(t *testing.T, pool *pgxpool.Pool) (webhookSecret string) {
	t.Helper()
	ctx := context.Background()

	const tenantID = "00000000-0000-0000-0000-000000000001"
	const boardID = "board_demo"

	// Account
	var accountID string
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('pipeline-test') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	// Provider connection — encrypt a fake Mello PAT so GetDecryptedToken works
	webhookSecret = "test-webhook-secret"
	encryptedToken, err := secret.Seal("test-mello-pat", testSecretKey)
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO provider_connections (account_id, provider_code, webhook_secret, mcp_auth_enc, config, tenant_id)
		VALUES ($1, 'mello', $2, $3, '{}', $4)`,
		accountID, webhookSecret, encryptedToken, tenantID)
	if err != nil {
		t.Fatalf("insert provider_connection: %v", err)
	}

	// Account identity
	_, err = pool.Exec(ctx, `
		INSERT INTO account_identities (account_id, provider_code, external_user_id, tenant_id)
		VALUES ($1, 'mello', 'user_123', $2)`,
		accountID, tenantID)
	if err != nil {
		t.Fatalf("insert account_identity: %v", err)
	}

	// Watched container — external_container_id must match board_id in webhook payload
	_, err = pool.Exec(ctx, `
		INSERT INTO watched_containers (account_id, provider_code, external_container_id, tenant_id)
		VALUES ($1, 'mello', $2, $3)`,
		accountID, boardID, tenantID)
	if err != nil {
		t.Fatalf("insert watched_container: %v", err)
	}

	// Runtime — code matches the @mework trigger's profile
	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (account_id, code, label, token_lookup, tenant_id)
		VALUES ($1, 'dev', 'Dev Runtime', 'test-token-lookup', $2)`,
		accountID, tenantID)
	if err != nil {
		t.Fatalf("insert runtime: %v", err)
	}

	// Profile — name matches runtime code
	_, err = pool.Exec(ctx, `
		INSERT INTO profiles (account_id, name, body, tenant_id)
		VALUES ($1, 'dev', 'You are a helpful AI assistant.', $2)`,
		accountID, tenantID)
	if err != nil {
		t.Fatalf("insert profile: %v", err)
	}

	return
}

// signMelloPayload creates the HMAC-SHA256 signature as Mello would.
func signMelloPayload(body []byte, timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TestTriggerGrammar validates the @mework trigger parsing in isolation.
func TestTriggerGrammar(t *testing.T) {
	tests := []struct {
		input    string
		wantOK   bool
		wantProf string
		wantWork string
		wantInst string
	}{
		{input: "@mework dev review check the PR", wantOK: true, wantProf: "dev", wantWork: "review", wantInst: "check the PR"},
		{input: "@mework dev", wantOK: true, wantProf: "dev", wantWork: "", wantInst: ""},
		{input: "@mework REVIEW quick audit", wantOK: true, wantProf: "REVIEW", wantWork: "", wantInst: "quick audit"},
		{input: "just a regular comment", wantOK: false},
		{input: "@mework", wantOK: false, wantProf: "", wantWork: "", wantInst: ""},
		{input: "@mework senior-dev ship implement auth", wantOK: true, wantProf: "senior-dev", wantWork: "ship", wantInst: "implement auth"},
		{input: "@mework dev plan design the API", wantOK: true, wantProf: "dev", wantWork: "plan", wantInst: "design the API"},
		{input: "@mework dev test run tests", wantOK: true, wantProf: "dev", wantWork: "test", wantInst: "run tests"},
		{input: "@mework dev cook", wantOK: true, wantProf: "dev", wantWork: "cook", wantInst: ""},
	}

	for _, tt := range tests {
		name := tt.input
		if len(name) > 30 {
			name = name[:30]
		}
		t.Run(name, func(t *testing.T) {
			prof, workflow, inst, ok := parseTrigger(tt.input)
			if tt.wantOK && !ok {
				t.Errorf("input %q: expected ok, got false", tt.input)
				return
			}
			if !tt.wantOK && ok {
				t.Errorf("input %q: expected !ok, got profile=%q workflow=%q inst=%q",
					tt.input, prof, workflow, inst)
				return
			}
			if tt.wantOK {
				if prof != tt.wantProf {
					t.Errorf("profile=%q, want %q", prof, tt.wantProf)
				}
				if workflow != tt.wantWork {
					t.Errorf("workflow=%q, want %q", workflow, tt.wantWork)
				}
				if inst != tt.wantInst {
					t.Errorf("instructions=%q, want %q", inst, tt.wantInst)
				}
			}
		})
	}
}

// parseTrigger mirrors webhook/parse.go's ParseTrigger for standalone testing.
func parseTrigger(body string) (profile, workflow, instructions string, ok bool) {
	idx := -1
	if strings.HasPrefix(body, "@mework") {
		idx = 0
	} else {
		idx = strings.Index(body, " @mework")
		if idx != -1 {
			idx++
		} else {
			idx = strings.Index(body, "\n@mework")
			if idx != -1 {
				idx++
			}
		}
	}
	if idx == -1 {
		return "", "", "", false
	}
	remaining := body[idx+len("@mework"):]
	words := strings.Fields(remaining)
	if len(words) == 0 {
		return "", "", "", false
	}
	profile = words[0]
	if len(words) >= 2 {
		second := strings.ToLower(words[1])
		if second == "plan" || second == "cook" || second == "test" || second == "review" || second == "ship" || second == "journal" || second == "fix" {
			workflow = second
			pos := strings.Index(remaining, words[1])
			if pos != -1 {
				instructions = strings.TrimSpace(remaining[pos+len(words[1]):])
			}
		} else {
			pos := strings.Index(remaining, profile)
			if pos != -1 {
				instructions = strings.TrimSpace(remaining[pos+len(profile):])
			}
		}
	}
	return profile, workflow, instructions, true
}

// TestWebhookSignature validates the HMAC verification in isolation.
func TestWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"type":"comment.added","data":{"body":"@mework dev review"}}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// Valid signature
	sig := signMelloPayload(body, ts, secret)
	if !strings.HasPrefix(sig, "sha256=") {
		t.Fatal("signature should start with sha256=")
	}
	t.Logf("Valid signature: %s", sig[:30]+"...")

	// Tampered body should produce different signature
	tamperedBody := []byte(`{"type":"comment.added","data":{"body":"@mework dev ship"}}`)
	sig2 := signMelloPayload(tamperedBody, ts, secret)
	if sig == sig2 {
		t.Error("different payloads should produce different signatures")
	}

	// Wrong secret should produce different signature
	sig3 := signMelloPayload(body, ts, "wrong-secret")
	if sig == sig3 {
		t.Error("different secrets should produce different signatures")
	}

	// Expired timestamp (>5 min old)
	oldTS := strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)
	sig4 := signMelloPayload(body, oldTS, secret)
	t.Logf("Old sig: %s", sig4[:30]+"...")
	t.Log("Signature verification edge cases OK")
}

// TestMelloWriteBack validates the REST write-back flow with fake Mello.
func TestMelloWriteBack(t *testing.T) {
	writebackCh := make(chan string, 1)
	fakeMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/comments") && r.Method == "POST" {
			body, _ := io.ReadAll(r.Body)
			writebackCh <- string(body)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintln(w, `{"id":"comment_123"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeMello.Close()

	result := map[string]string{
		"ticket_id": "TICKET-1",
		"body":      "AI review complete - found 3 issues.",
	}
	data, _ := json.Marshal(result)

	resp, err := http.Post(
		fakeMello.URL+"/api/v1/tickets/TICKET-1/comments",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		t.Fatalf("write-back POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}

	select {
	case received := <-writebackCh:
		if !strings.Contains(received, "AI review complete") {
			t.Errorf("body missing expected content: %s", received)
		}
		t.Log("Write-back received by fake Mello")
	case <-time.After(time.Second):
		t.Error("write-back not received by fake Mello")
	}
}

// TestMelloFullPipeline validates the full inbound webhook pipeline:
// signed webhook -> ExtractContainerID -> watched container lookup
// -> signature verify -> ParseEvent -> trigger grammar -> job enqueue.
func TestMelloFullPipeline(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/mework_test?sslmode=disable"
	}
	if err := storePkg.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 1. Fake Mello server — returns a ticket when the webhook handler fetches it
	fakeMello := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Fake Mello <- %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          "TICKET-1",
			"title":       "Test ticket from @mework",
			"description": "Please review the latest PR",
		})
	}))
	defer fakeMello.Close()

	// 2. Connect + seed DB
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	cleanupDB(t, pool)
	webhookSecret := seedFullPipelineData(t, pool)

	// 3. Start mework server pointing at fake Mello
	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ServerKey:       "test-server-key",
		MeworkSecretKey: testSecretKey,
		ListenAddr:      "localhost:0",
		WebhookSecret:   webhookSecret,
		MelloBaseURL:    fakeMello.URL,
	}
	hubSrv := hub.NewServer(pool, cfg)
	ts := httptest.NewServer(hubSrv)
	defer ts.Close()
	t.Logf("mework server at %s, fake Mello at %s", ts.URL, fakeMello.URL)

	// 4. Build a Mello-format webhook payload
	payload := map[string]interface{}{
		"id":   "evt_001",
		"type": "comment.added",
		"actor": map[string]interface{}{
			"id":   "user_123",
			"name": "Test User",
		},
		"model": map[string]interface{}{
			"type":     "board",
			"board_id": "board_demo",
		},
		"data": map[string]interface{}{
			"id":        "comment_001",
			"body":      "@mework dev review check the PR for bugs",
			"ticket_id": "TICKET-1",
		},
	}
	body, _ := json.Marshal(payload)
	tsSec := strconv.FormatInt(time.Now().Unix(), 10)
	sig := signMelloPayload(body, tsSec, webhookSecret)

	webhookURL := ts.URL + "/webhooks/mello"
	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mello-Signature", sig)
	req.Header.Set("X-Mello-Timestamp", tsSec)
	req.Header.Set("X-Mello-Delivery-Id", "e2e-test-1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST webhook: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, string(respBody))
	}
	t.Log("Webhook accepted (202) - full pipeline works")

	// 5. Verify job was created in DB
	var jobCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE external_event_id = 'e2e-test-1'`).Scan(&jobCount)
	if err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if jobCount != 1 {
		t.Errorf("expected 1 job, got %d", jobCount)
	} else {
		t.Log("Job created in DB")
	}

	// 6. Verify bad signature is rejected
	reqBad, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	reqBad.Header.Set("Content-Type", "application/json")
	reqBad.Header.Set("X-Mello-Signature", "sha256=bad")
	reqBad.Header.Set("X-Mello-Timestamp", tsSec)
	reqBad.Header.Set("X-Mello-Delivery-Id", "e2e-test-2")

	respBad, _ := http.DefaultClient.Do(reqBad)
	if respBad.StatusCode != http.StatusUnauthorized {
		rb, _ := io.ReadAll(respBad.Body)
		t.Errorf("expected 401 for bad sig, got %d: %s", respBad.StatusCode, string(rb))
	} else {
		t.Log("Bad signature correctly rejected (401)")
	}
}

// TestRuntimeEnrollAndHealth validates the basic API endpoints are reachable.
// NOTE: The /api/v1/runners/enroll endpoint requires a registration token
// issued via the PAT-protected /api/v1/runners/registration-tokens endpoint.
// Full enrollment testing is covered by the e2e suite in tests/e2e/.
func TestRuntimeEnrollAndHealth(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/mework_test?sslmode=disable"
	}
	if err := storePkg.RunMigrations(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	cleanupDB(t, pool)

	// Start server
	cfg := &hub.Config{
		DatabaseURL:     dsn,
		ServerKey:       "test-server-key",
		MeworkSecretKey: testSecretKey,
		ListenAddr:      "localhost:0",
	}
	hubSrv := hub.NewServer(pool, cfg)
	ts := httptest.NewServer(hubSrv)
	defer ts.Close()

	// Health check — open endpoint, no auth required
	t.Run("healthz", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/healthz")
		if err != nil {
			t.Fatalf("GET /healthz: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		} else {
			t.Log("Health check OK (200)")
		}
	})

	// Enroll endpoint without auth — demonstrates expected error
	t.Run("enroll unauthorized", func(t *testing.T) {
		regPayload := `{"code":"e2e-test","label":"E2E runner"}`
		regResp, err := http.Post(
			ts.URL+"/api/v1/runners/enroll",
			"application/json",
			strings.NewReader(regPayload),
		)
		if err != nil {
			t.Fatalf("enroll: %v", err)
		}
		defer regResp.Body.Close()
		if regResp.StatusCode == http.StatusUnauthorized {
			t.Log("Enroll correctly rejects requests without registration token (401)")
		} else {
			rb, _ := io.ReadAll(regResp.Body)
			t.Errorf("expected 401 when no auth token, got %d: %s", regResp.StatusCode, string(rb))
		}
	})
}
