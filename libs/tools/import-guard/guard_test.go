package guard_test

import (
	"testing"

	guard "mework/tools/import-guard"
)

func TestCheckImport_ModuleBoundaryGuard(t *testing.T) {
	tests := []struct {
		name       string
		sourceMod  string // the importing module's path
		importPath string // the import being checked
		allowed    bool   // whether the guard should allow it
	}{
		// ---- Positive cases: these imports MUST be allowed ----
		{
			name:       "shared imports stdlib",
			sourceMod:  "mework/shared",
			importPath: "fmt",
			allowed:    true,
		},
		{
			name:       "server imports shared/core",
			sourceMod:  "mework/server",
			importPath: "mework/libs/shared/core",
			allowed:    true,
		},
		{
			name:       "client imports shared/ports",
			sourceMod:  "mework/client",
			importPath: "mework/libs/shared/ports",
			allowed:    true,
		},
		{
			name:       "client imports sandbox/engine/local",
			sourceMod:  "mework/client",
			importPath: "mework/libs/sandbox/engine/local",
			allowed:    true,
		},
		{
			name:       "sandbox imports shared/ports",
			sourceMod:  "mework/sandbox",
			importPath: "mework/libs/shared/ports",
			allowed:    true,
		},

		// ---- Negative cases: these imports MUST be rejected ----
		{
			name:       "shared importing server/hub is forbidden",
			sourceMod:  "mework/shared",
			importPath: "mework/libs/server/hub",
			allowed:    false,
		},
		{
			name:       "server importing client/cli is forbidden",
			sourceMod:  "mework/server",
			importPath: "mework/libs/client/cli",
			allowed:    false,
		},
		{
			name:       "server importing sandbox/runtime is forbidden",
			sourceMod:  "mework/server",
			importPath: "mework/libs/sandbox/runtime",
			allowed:    false,
		},
		{
			name:       "client importing server/hub is forbidden",
			sourceMod:  "mework/client",
			importPath: "mework/libs/server/hub",
			allowed:    false,
		},
		{
			name:       "sandbox importing server/hub is forbidden",
			sourceMod:  "mework/sandbox",
			importPath: "mework/libs/server/hub",
			allowed:    false,
		},
		{
			name:       "engine/local importing engine/docker is forbidden",
			sourceMod:  "mework/libs/sandbox/engine/local",
			importPath: "mework/libs/sandbox/engine/docker",
			allowed:    false,
		},
		{
			name:       "shared importing heavy third-party dep is forbidden",
			sourceMod:  "mework/shared",
			importPath: "github.com/docker/docker/client",
			allowed:    false,
		},
		{
			name:       "client importing server/platform is forbidden",
			sourceMod:  "mework/client",
			importPath: "mework/libs/server/platform/store",
			allowed:    false,
		},
		{
			name:       "sandbox importing client/cli is forbidden",
			sourceMod:  "mework/sandbox",
			importPath: "mework/libs/client/cli",
			allowed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guard.CheckImport(tt.sourceMod, tt.importPath)
			if got != tt.allowed {
				t.Errorf("CheckImport(%q, %q) = %v, want %v",
					tt.sourceMod, tt.importPath, got, tt.allowed)
			}
		})
	}
}
