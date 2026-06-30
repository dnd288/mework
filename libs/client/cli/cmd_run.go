package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"mework/libs/client/runner"
)

var runWorkspaceDir string

// runCmd sends a one-shot instruction to the running offline-mode agent.
var runCmd = &cobra.Command{
	Use:   "run <instruction>",
	Short: "Send a one-shot instruction to the running offline-mode agent",
	Long: `Send a one-shot instruction to the running offline-mode agent.

The instruction is passed to the agent over stdin (never argv) to preserve
the injection-safety invariant. Requires a running offline agent started
with 'mework daemon start --offline --workspace <dir>'.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "" {
			return fmt.Errorf("instruction is required")
		}
		instruction := strings.Join(args, " ")

		wsDir := runWorkspaceDir
		if wsDir == "" {
			var err error
			wsDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("get workspace directory: %w", err)
			}
		}

		sockPath, err := runner.SocketPath(wsDir)
		if err != nil {
			return fmt.Errorf("socket path: %w", err)
		}

		if !runner.CheckAgentRunning(sockPath) {
			return fmt.Errorf("no offline agent running — run 'mework daemon start --offline' first")
		}

		output, exitCode, err := runner.SendInstructionResult(sockPath, instruction)
		if err != nil {
			return err
		}

		if output != "" {
			fmt.Fprint(cmd.OutOrStdout(), output)
		}

		if exitCode != 0 {
			return fmt.Errorf("task failed with exit code %d", exitCode)
		}
		return nil
	},
}

func init() {
	runCmd.Flags().StringVar(&runWorkspaceDir, "workspace", "", "workspace directory for offline mode (default: current directory)")
}
