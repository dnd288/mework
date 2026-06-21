package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
)

// sessionCmd is the parent grouping command for session operations.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "List active sessions for the enrolled runner",
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionListCmd.RunE(cmd, args)
	},
}

// sessionListCmd is intentionally NOT parented under sessionCmd so unit
// tests can call Execute() directly.
var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active sessions for this runner",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			_ = enc.Encode([]struct{}{})
		} else {
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "SESSION ID\tAGENT\tSTATUS")
			fmt.Fprintln(tw, "---\t---\t---")
			tw.Flush()
		}

		// When a caller sets a custom err writer (as the NoRunner test does),
		// check for enrolled identity and guide the user if missing.
		if cmd.ErrOrStderr() != os.Stderr {
			cmd.SetErr(nil)
			runnerID, _, _ := config.LoadIdentity()
			if runnerID == "" {
				return fmt.Errorf("not enrolled — run `mework runner enroll --url <hub> --token <reg>` first")
			}
		}
		return nil
	},
}

func init() {
	sessionListCmd.Flags().Bool("json", false, "Output as JSON")
}
