package main

import (
	"testing"
)

// TestPack_CreatesValidBundleZip verifies that PackBundle creates a valid
// sandbox bundle zip from a source directory.
// RED: PackBundle is not yet implemented — this test fails to compile.
func TestPack_CreatesValidBundleZip(t *testing.T) {
	zipPath, err := PackBundle("/tmp/testdata/valid-sandbox", "/tmp/out.zip")
	if err != nil {
		t.Fatalf("PackBundle failed: %v", err)
	}
	if zipPath == "" {
		t.Error("expected non-empty zip output path")
	}
}

// TestPack_ValidatesDirectory verifies that PackBundle rejects a directory
// missing the required sandbox.yaml file.
// RED: PackBundle is not yet implemented — this test fails to compile.
func TestPack_ValidatesDirectory(t *testing.T) {
	_, err := PackBundle("/tmp/testdata/no-sandbox-yaml", "/tmp/out.zip")
	if err == nil {
		t.Error("expected error for directory without sandbox.yaml, got nil")
	}
}
