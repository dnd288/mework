package shared_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// forbiddenImports lists module path prefixes that must never appear
// in the import graph of mework/shared.
var forbiddenImports = []string{
	"mework/client",
	"mework/server",
	"mework/sandbox",
	"internal/",
}

func TestSharedModule_NoForbiddenImports(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "./...")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed (module may not exist yet): %v", err)
	}

	var pkg struct {
		ImportPath string
		Imports    []string
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for {
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		for _, imp := range pkg.Imports {
			for _, forbid := range forbiddenImports {
				if strings.HasPrefix(imp, forbid) {
					t.Errorf("package %q imports forbidden %q", pkg.ImportPath, imp)
				}
			}
		}
	}
}
