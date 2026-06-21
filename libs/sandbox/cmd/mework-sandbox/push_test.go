package main

import (
	"testing"
)

// TestPush_ToCatalog verifies that PushBundle publishes a bundle to the
// agent catalog's PublishVersion endpoint.
// RED: PushBundle is not yet implemented — this test fails to compile.
func TestPush_ToCatalog(t *testing.T) {
	err := PushBundle("http://localhost:8080", "test-agent", "1.0.0", "/tmp/test-sandbox.zip")
	if err != nil {
		t.Fatalf("PushBundle failed: %v", err)
	}
}

// TestPush_ValidatesBeforeUpload verifies that PushBundle rejects a bundle
// with missing required metadata before making an HTTP call.
// RED: PushBundle is not yet implemented — this test fails to compile.
func TestPush_ValidatesBeforeUpload(t *testing.T) {
	err := PushBundle("http://localhost:8080", "", "", "/tmp/invalid.zip")
	if err == nil {
		t.Error("expected error for empty name and version, got nil")
	}
}
