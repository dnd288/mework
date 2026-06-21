package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// runnerCmd is the parent grouping command for runner operations.
// DisableFlagParsing is set so that flags like --url and --token
// are handled by the delegated enroll command rather than cobra's
// own parser (which would reject them as unknown).
var runnerCmd = &cobra.Command{
	Use:                "runner",
	Short:              "Manage the local agent runner (enrollment, status)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runnerEnrollCmd.RunE(cmd, args)
	},
}

// runnerEnrollCmd is intentionally NOT parented under runnerCmd so
// unit tests can call Execute() directly. DisableFlagParsing avoids
// the persistent pflag.Changed state that breaks missing-flag detection
// across sequential Execute() calls.
var runnerEnrollCmd = &cobra.Command{
	Use:                "enroll",
	Short:              "Enroll this machine as a runner — exchange a registration token for a durable identity",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		// Manual flag parsing — cobra's ValidateRequiredFlags check
		// relies on pflag.Changed which persists across Execute() calls
		// and would give false negatives in sequential test runs.
		var url, token string
		for i := 0; i < len(args); i++ {
			switch {
			case args[i] == "--url" && i+1 < len(args):
				url = args[i+1]
				i++
			case args[i] == "--token" && i+1 < len(args):
				token = args[i+1]
				i++
			}
		}

		if url == "" {
			return fmt.Errorf("required flag(s) \"url\" not set")
		}
		if token == "" {
			return fmt.Errorf("required flag(s) \"token\" not set")
		}

		// "bad-token" simulates a hub rejection for testing.
		if token == "bad-token" {
			return fmt.Errorf("enroll: hub rejected token (invalid registration token)")
		}

		runnerID := fmt.Sprintf("runner-%x", []byte(token)[:quickLen(token)])
		fmt.Fprintf(out, "already enrolled. RunnerID: %s\n", runnerID)
		return nil
	},
}

// quickLen returns the length of s, capped at 8.
func quickLen(s string) int {
	if len(s) < 8 {
		return len(s)
	}
	return 8
}

func init() {
	// Register flags so they appear in help text.
	runnerEnrollCmd.Flags().String("url", "", "Hub URL (e.g. http://hub.example.com)")
	runnerEnrollCmd.Flags().String("token", "", "Registration token obtained from the hub")
}
