package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var initWorkspace string
var initRole string
var initBackend string
var initName string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a mework workspace from a template",
	Long: `Initialize a directory as a mework workspace.

Creates mework.yml, CLAUDE.md, .claude/settings.json with MCP config,
and optional .claude/skills/ and .claude/commands/ based on the selected
agent role.

Examples:
  mework init --workspace . --agent orchestrator --name mybot
  mework init --workspace ./my-project --agent worker
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := initWorkspace
		if dir == "" {
			dir = "."
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		// Find the template.
		tmplDir := findWorkspaceTemplate(initRole)
		if tmplDir == "" {
			return fmt.Errorf("template for agent %q not found", initRole)
		}

		// Copy template files.
		mcpBin := resolveMCPBinPath()
		if err := copyWorkspaceTemplate(tmplDir, absDir, mcpBin); err != nil {
			return fmt.Errorf("copy template: %w", err)
		}

		// Create mework.yml at root.
		name := initName
		if name == "" {
			name = initRole
		}
		yml := fmt.Sprintf(`name: %s
version: "1.0.0"
engine: local
backend: %s
role: %s
`, name, initBackend, initRole)
		if err := os.WriteFile(absDir+"/mework.yml", []byte(yml), 0600); err != nil {
			return fmt.Errorf("write mework.yml: %w", err)
		}

		// Init git repo so Claude discovers project-level MCP config.
		if _, err := os.Stat(absDir + "/.git"); os.IsNotExist(err) {
			gitCmd := exec.Command("git", "init")
			gitCmd.Dir = absDir
			if out, err := gitCmd.CombinedOutput(); err != nil {
				fmt.Printf("warning: git init failed: %v\n%s\n", err, out)
			} else {
				// Make an initial commit so CLAUDE.md is tracked.
				exec.Command("git", "-C", absDir, "add", "-A").Run()
				exec.Command("git", "-C", absDir, "commit", "-m", "init mework workspace").Run()
			}
		}

		fmt.Printf("mework workspace initialized: %s\n", absDir)
		fmt.Printf("  role:     %s\n", initRole)
		fmt.Printf("  backend:  %s\n", initBackend)
		fmt.Printf("  commands: /sessions, /spawn, /status, /stop\n")
		fmt.Println()
		fmt.Println("To start the orchestrator agent:")
		fmt.Println("  mework-mezon-worker")
		fmt.Println()
		fmt.Println("For offline mode (local agent, no server):")
		fmt.Println("  mework daemon start --offline --workspace .")
		return nil
	},
}

// findWorkspaceTemplate locates the template directory for a given role.
func findWorkspaceTemplate(role string) string {
	rel := "templates/workspace/" + role
	candidates := []string{
		rel,
	}
	// Check relative to the mework binary.
	if exe, err := os.Executable(); err == nil {
		if idx := strings.LastIndex(exe, "/"); idx >= 0 {
			base := exe[:idx]
			candidates = append(candidates,
				base+"/"+rel,
				base+"/../"+rel,
				base+"/../../"+rel,
			)
		}
	}
	// Check relative to HOME.
	if home := os.Getenv("HOME"); home != "" {
		candidates = append(candidates,
			home+"/Documents/mework/"+rel,
			home+"/mework/"+rel,
		)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return ""
}

// resolveMCPBinPath finds the mework-mcp binary.
func resolveMCPBinPath() string {
	if exe, err := os.Executable(); err == nil {
		if idx := strings.LastIndex(exe, "/"); idx >= 0 {
			base := exe[:idx]
			for _, c := range []string{
				base + "/mework-mcp",
				base + "/../bin/mework-mcp",
				base + "/../../bin/mework-mcp",
			} {
				if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
					return c
				}
			}
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		if fi, err := os.Stat(home + "/go/bin/mework-mcp"); err == nil && !fi.IsDir() {
			return home + "/go/bin/mework-mcp"
		}
	}
	return "mework-mcp"
}

// copyWorkspaceTemplate copies template files, replacing __MCP_BIN_PATH__.
func copyWorkspaceTemplate(src, dst, mcpBin string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return nil
		}
		dstPath := dst + "/" + rel
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ReplaceAll(string(data), "__MCP_BIN_PATH__", mcpBin)
		_ = os.MkdirAll(filepath.Dir(dstPath), 0700)
		return os.WriteFile(dstPath, []byte(content), 0600)
	})
}

func init() {
	initCmd.Flags().StringVar(&initWorkspace, "workspace", "", "Target directory (default: current dir)")
	initCmd.Flags().StringVar(&initRole, "agent", "orchestrator", "Agent role: orchestrator or worker")
	initCmd.Flags().StringVar(&initName, "name", "", "Agent name for mework agent send (default: same as --agent)")
	initCmd.Flags().StringVar(&initBackend, "backend", "claude", "AI backend (claude, codex, etc.)")
}
