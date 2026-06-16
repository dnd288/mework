package server

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Backup existing env
	oldDB := os.Getenv("DATABASE_URL")
	oldAddr := os.Getenv("LISTEN_ADDR")
	oldSecret := os.Getenv("WEBHOOK_SECRET")
	oldKey := os.Getenv("SERVER_KEY")
	oldMeworkSecret := os.Getenv("MEWORK_SECRET_KEY")
	oldMello := os.Getenv("MELLO_BASE_URL")

	defer func() {
		os.Setenv("DATABASE_URL", oldDB)
		os.Setenv("LISTEN_ADDR", oldAddr)
		os.Setenv("WEBHOOK_SECRET", oldSecret)
		os.Setenv("SERVER_KEY", oldKey)
		os.Setenv("MEWORK_SECRET_KEY", oldMeworkSecret)
		os.Setenv("MELLO_BASE_URL", oldMello)
	}()

	// Clear variables for test
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("LISTEN_ADDR")
	os.Unsetenv("WEBHOOK_SECRET")
	os.Unsetenv("SERVER_KEY")
	os.Unsetenv("MEWORK_SECRET_KEY")
	os.Unsetenv("MELLO_BASE_URL")

	// Test missing DATABASE_URL
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}

	// Set DATABASE_URL, test missing SERVER_KEY
	os.Setenv("DATABASE_URL", "postgres://localhost:5432/mework_test")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected error when SERVER_KEY is missing")
	}

	// Set SERVER_KEY, test missing MEWORK_SECRET_KEY
	os.Setenv("SERVER_KEY", "super-secret-server-key-hmac-sha256-hash")
	_, err = LoadConfig()
	if err == nil {
		t.Fatal("expected error when MEWORK_SECRET_KEY is missing")
	}

	// Set both
	os.Setenv("MEWORK_SECRET_KEY", "mework-secret-key-aes-256")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading valid config: %v", err)
	}

	if cfg.DatabaseURL != "postgres://localhost:5432/mework_test" {
		t.Errorf("expected DatabaseURL to match env, got: %s", cfg.DatabaseURL)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("expected default ListenAddr to be :8080, got: %s", cfg.ListenAddr)
	}
	if cfg.ServerKey != "super-secret-server-key-hmac-sha256-hash" {
		t.Errorf("expected ServerKey to match env, got: %s", cfg.ServerKey)
	}

	// Test custom ListenAddr and WebhookSecret
	os.Setenv("LISTEN_ADDR", ":9090")
	os.Setenv("WEBHOOK_SECRET", "mello-webhook-secret-token")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Errorf("expected ListenAddr to match env, got: %s", cfg.ListenAddr)
	}
	if cfg.WebhookSecret != "mello-webhook-secret-token" {
		t.Errorf("expected WebhookSecret to match env, got: %s", cfg.WebhookSecret)
	}
}
