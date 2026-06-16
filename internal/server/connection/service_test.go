package connection

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mework/internal/store"
)

func TestConnectionService(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Clear DB
	_, err = pool.Exec(ctx, "DELETE FROM watched_containers; DELETE FROM account_identities; DELETE FROM provider_connections; DELETE FROM accounts;")
	if err != nil {
		t.Fatalf("failed to clean db: %v", err)
	}

	// Insert test accounts
	var accountID1 string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Account 1') RETURNING id").Scan(&accountID1)
	if err != nil {
		t.Fatalf("failed to insert account: %v", err)
	}

	var accountID2 string
	err = pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Account 2') RETURNING id").Scan(&accountID2)
	if err != nil {
		t.Fatalf("failed to insert account 2: %v", err)
	}

	secretKey := "mysecretkey"
	svc := NewService(pool, secretKey)

	// 1. Create a connection
	conn1, err := svc.CreateConnection(ctx, accountID1, "mello", "mello_pat_token_xyz", "webhooksecret123", map[string]any{"workspace_id": "ws_1"})
	if err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}

	if conn1.ProviderCode != "mello" {
		t.Errorf("expected provider_code mello, got: %s", conn1.ProviderCode)
	}

	if conn1.WebhookSecret != "webhooksecret123" {
		t.Errorf("expected webhook secret, got: %s", conn1.WebhookSecret)
	}

	// 2. Verify encrypted field at rest in DB
	var dbToken string
	err = pool.QueryRow(ctx, "SELECT mcp_auth_enc FROM provider_connections WHERE id = $1", conn1.ID).Scan(&dbToken)
	if err != nil {
		t.Fatalf("failed to query encrypted token: %v", err)
	}

	if dbToken == "mello_pat_token_xyz" {
		t.Error("expected database token to be encrypted, got plaintext")
	}

	// 3. Get connection (does not return token)
	gotConn, err := svc.GetConnection(ctx, accountID1, "mello")
	if err != nil {
		t.Fatalf("failed to get connection: %v", err)
	}

	if gotConn.ID != conn1.ID {
		t.Errorf("expected connection ID %s, got %s", conn1.ID, gotConn.ID)
	}

	// 4. Decrypt token internally (should succeed and match)
	decryptedToken, err := svc.GetDecryptedToken(ctx, accountID1, "mello")
	if err != nil {
		t.Fatalf("failed to decrypt token: %v", err)
	}
	if decryptedToken != "mello_pat_token_xyz" {
		t.Errorf("expected decrypted token mello_pat_token_xyz, got %s", decryptedToken)
	}

	// 5. Cross-account access check (should fail with ErrNotFound)
	_, err = svc.GetConnection(ctx, accountID2, "mello")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account read, got: %v", err)
	}

	_, err = svc.GetDecryptedToken(ctx, accountID2, "mello")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account token decrypt, got: %v", err)
	}

	// 6. Delete connection
	err = svc.DeleteConnection(ctx, accountID2, "mello") // IDOR delete check
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for cross-account delete, got: %v", err)
	}

	err = svc.DeleteConnection(ctx, accountID1, "mello")
	if err != nil {
		t.Fatalf("failed to delete connection: %v", err)
	}

	_, err = svc.GetConnection(ctx, accountID1, "mello")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after deletion, got: %v", err)
	}
}
