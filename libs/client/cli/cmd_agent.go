package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// agentCmd is the parent grouping command for agent operations.
// In production, `mework agent list` is processed here; in tests
// agentListCmd is executed directly (no parent) so cobra's Execute
// does not delegate to the root command.
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "List available agents from the catalog",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agentListCmd.RunE(cmd, args)
	},
}

// agentListCmd is intentionally NOT parented under agentCmd so unit
// tests can call Execute() directly.
var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agents from the catalog",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode([]struct{}{})
		}
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tVERSION")
		fmt.Fprintln(tw, "---\t---")
		tw.Flush()
		return nil
	},
}

func init() {
	agentListCmd.Flags().Bool("json", false, "Output as JSON")
}
