package server

import (
	"os/exec"
	"testing"
)

func TestServerPackagesExist(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
	}{
		{name: "hub", pkgPath: "mework/libs/server/hub"},
		{name: "registry", pkgPath: "mework/libs/server/registry"},
		{name: "session", pkgPath: "mework/libs/server/session"},
		{name: "catalog", pkgPath: "mework/libs/server/catalog"},
		{name: "orchestrator", pkgPath: "mework/libs/server/orchestrator"},
		{name: "webhook", pkgPath: "mework/libs/server/webhook"},
		{name: "writeback", pkgPath: "mework/libs/server/writeback"},
		{name: "auth", pkgPath: "mework/libs/server/auth"},
		{name: "middleware", pkgPath: "mework/libs/server/middleware"},
		{name: "platform/store", pkgPath: "mework/libs/server/platform/store"},
		{name: "platform/secret", pkgPath: "mework/libs/server/platform/secret"},
		{name: "platform/token", pkgPath: "mework/libs/server/platform/token"},
		{name: "bus", pkgPath: "mework/libs/server/bus"},
		{name: "storage", pkgPath: "mework/libs/server/storage"},
		{name: "provider", pkgPath: "mework/libs/server/provider"},
		{name: "cmd/mework-server", pkgPath: "mework/libs/server/cmd/mework-server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-find", tt.pkgPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("package %q should resolve but go list failed: %v\noutput: %s", tt.pkgPath, err, string(out))
				return
			}
			if len(out) == 0 {
				t.Errorf("package %q should resolve but go list returned empty", tt.pkgPath)
			}
		})
	}
}
