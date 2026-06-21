package sandbox

import (
	"os/exec"
	"testing"
)

func TestSandboxPackagesExist(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
	}{
		{name: "runtime", pkgPath: "mework/libs/sandbox/runtime"},
		{name: "engine/local", pkgPath: "mework/libs/sandbox/engine/local"},
		{name: "agent", pkgPath: "mework/libs/sandbox/agent"},
		{name: "engine/docker", pkgPath: "mework/libs/sandbox/engine/docker"},
		{name: "engine/cloudflare", pkgPath: "mework/libs/sandbox/engine/cloudflare"},
		{name: "engine/custom", pkgPath: "mework/libs/sandbox/engine/custom"},
		{name: "cmd/mework-sandbox", pkgPath: "mework/libs/sandbox/cmd/mework-sandbox"},
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
