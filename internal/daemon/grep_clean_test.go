package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepClean(t *testing.T) {
	// Traverse daemon package directory and check that no file imports mello or mcp packages
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("failed to glob go files: %v", err)
	}

	forbidden := []string{
		"mework/internal/mello",
		"mework/internal/mcp",
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue // skip test files
		}

		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", file, err)
		}

		for _, f := range forbidden {
			if strings.Contains(string(content), `"`+f+`"`) {
				t.Errorf("file %s violates isolation: imports forbidden package %q", file, f)
			}
		}
	}
}
