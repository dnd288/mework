package main

import (
	"context"
	"testing"
)

// TestConnectRedis_UsesMiniredisWhenURLIsEmpty verifies that connectRedis
// starts an embedded miniredis when RedisURL is empty, returning a working client.
// RED: this fails because connectRedis is not yet implemented.
func TestConnectRedis_UsesMiniredisWhenURLIsEmpty(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{RedisURL: ""}

	rdb, err := connectRedis(ctx, cfg)
	if err != nil {
		t.Fatalf("connectRedis() with empty URL should not error, got: %v", err)
	}
	if rdb == nil {
		t.Fatal("connectRedis() returned nil Cmdable")
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping() on miniredis-backed client should succeed: %v", err)
	}
}

// TestConnectRedis_ReturnsErrorForBadURL verifies that connectRedis returns
// an error when given an invalid Redis URL.
// RED: this fails because connectRedis is not yet implemented.
func TestConnectRedis_ReturnsErrorForBadURL(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{RedisURL: "redis://!bad!"}

	rdb, err := connectRedis(ctx, cfg)
	if err == nil {
		t.Fatal("connectRedis() with bad URL should return error, got nil")
	}
	if rdb != nil {
		t.Errorf("connectRedis() returned non-nil Cmdable on error: %v", rdb)
	}
}
