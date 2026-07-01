package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type moduleConfig struct {
	name             string
	dir              string
	forbiddenImports []string
}

func TestModuleImportDAG(t *testing.T) {
	// Locate go.work from the test's own module root.
	gwDir := findGOWORKDir()
	gowork := filepath.Join(gwDir, "go.work")

	modules := []moduleConfig{
		{
			name:             "shared",
			dir:              "../shared",
			forbiddenImports: []string{"mework/server", "mework/client", "mework/sandbox"},
		},
		{
			name:             "server",
			dir:              "../server",
			forbiddenImports: []string{"mework/client", "mework/sandbox"},
		},
		{
			name:             "sandbox",
			dir:              "../sandbox",
			forbiddenImports: []string{"mework/server", "mework/client"},
		},
		{
			name:             "client",
			dir:              "../client",
			forbiddenImports: []string{"mework/server"},
		},
	}

	for _, mod := range modules {
		t.Run(mod.name, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-json", mod.dir+"/...")
			cmd.Env = append(os.Environ(), "GOWORK="+gowork)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("go list failed for module %q (module may not exist yet): %v", mod.name, err)
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
					for _, forbid := range mod.forbiddenImports {
						if strings.HasPrefix(imp, forbid) {
							t.Errorf("package %q imports forbidden %q", pkg.ImportPath, imp)
						}
					}
				}
			}
		})
	}
}

// findGOWORKDir walks up from the test's cwd (tests/) looking for go.work.
func findGOWORKDir() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}
