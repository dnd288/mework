package main

import (
	"os"
	"testing"
)

func TestLoad_WithoutRedisURL(t *testing.T) {
	// Ensure REDIS_URL is not set.
	os.Unsetenv("REDIS_URL")

	// Set required env vars for Load() to succeed.
	os.Setenv("MEZON_APP_ID", "test-app")
	os.Setenv("MEZON_API_KEY", "test-key")
	os.Setenv("MEWORK_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should succeed without REDIS_URL: %v", err)
	}
	if cfg.RedisURL != "" {
		t.Errorf("RedisURL = %q, want empty", cfg.RedisURL)
	}

	// Clean up env vars we set.
	defer func() {
		os.Unsetenv("MEZON_APP_ID")
		os.Unsetenv("MEZON_API_KEY")
		os.Unsetenv("MEWORK_TOKEN")
	}()
}

func TestLoad_WithRedisURL(t *testing.T) {
	wantURL := "redis://localhost:6379"
	os.Setenv("REDIS_URL", wantURL)

	os.Setenv("MEZON_APP_ID", "test-app")
	os.Setenv("MEZON_API_KEY", "test-key")
	os.Setenv("MEWORK_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should succeed with REDIS_URL: %v", err)
	}
	if cfg.RedisURL != wantURL {
		t.Errorf("RedisURL = %q, want %q", cfg.RedisURL, wantURL)
	}

	defer func() {
		os.Unsetenv("REDIS_URL")
		os.Unsetenv("MEZON_APP_ID")
		os.Unsetenv("MEZON_API_KEY")
		os.Unsetenv("MEWORK_TOKEN")
	}()
}

func TestLoad_RedisURLDefaultsToEmpty(t *testing.T) {
	os.Unsetenv("REDIS_URL")

	os.Setenv("MEZON_APP_ID", "test-app")
	os.Setenv("MEZON_API_KEY", "test-key")
	os.Setenv("MEWORK_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should succeed: %v", err)
	}
	if cfg.RedisURL != "" {
		t.Errorf("RedisURL = %q, want empty (zero value)", cfg.RedisURL)
	}

	defer func() {
		os.Unsetenv("MEZON_APP_ID")
		os.Unsetenv("MEZON_API_KEY")
		os.Unsetenv("MEWORK_TOKEN")
	}()
}
