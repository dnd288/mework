package main

import (
	"os"
	"path/filepath"
)

func init() {
	// Create test data directories used by pack_test.go and push_test.go.
	dirs := []string{
		"/tmp/testdata/valid-sandbox",
		"/tmp/testdata/no-sandbox-yaml",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			panic("failed to create test data dir " + d + ": " + err.Error())
		}
	}

	// Write required files for the valid-sandbox test fixture.
	validDir := dirs[0]
	_ = os.WriteFile(filepath.Join(validDir, "sandbox.yaml"),
		[]byte("name: test-agent\nversion: 1.0.0\nspec: claude-code\nbackend: local\nauthor: test\n"), 0644)
	_ = os.WriteFile(filepath.Join(validDir, "definition.md"),
		[]byte("# Test agent"), 0644)
}
