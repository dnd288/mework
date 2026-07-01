package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/spf13/cobra"

	"mework/libs/client/catalog"
	"mework/libs/shared/config"
)

// sandboxCmd is the workspace-oriented façade over the session API: it turns a
// local workspace folder into a server-addressable worker run by the local
// daemon. The lower-level `session` group remains for power users.
var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Run a local workspace as a server-addressable worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		return sandboxListCmd.RunE(cmd, args)
	},
}

// sandboxStartCmd validates a workspace, targets the local enrolled runner, and
// creates a workspace-bound session so the local daemon opens a sandbox bound to
// the folder. It prints the session id (or the full row with --json), and may
// stream events with --attach.
var sandboxStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Turn the workspace into a running worker (default: current dir)",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		dir, _ := cmd.Flags().GetString("workspace")
		if dir == "" {
			dir = "."
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}

		// Validate the workspace locally first: a missing/corrupt mework.yml
		// must fail before any network call so errors are immediate and the
		// daemon receives a real, resolvable path.
		meta, err := catalog.LoadWorkspaceConfig(abs)
		if err != nil {
			return err
		}

		// Target the local enrolled runner so the *local* daemon opens the
		// sandbox. No identity means the machine is not enrolled.
		runnerID, _, err := config.LoadIdentity()
		if err != nil {
			return err
		}
		if runnerID == "" {
			return fmt.Errorf("not enrolled — run `mework runner enroll` and `mework daemon start` first")
		}

		base, token, err := sessionEndpoint()
		if err != nil {
			return err
		}

		body := map[string]string{
			"agent_name": meta.Name,
			"runner":     runnerID,
			"workspace":  abs,
		}
		if meta.Version != "" {
			body["version"] = meta.Version
		}

		var created sessionRow
		if _, err := sessionDo(http.MethodPost, base+"/api/v1/sessions", token, body, &created); err != nil {
			return err
		}

		showJSON, _ := cmd.Flags().GetBool("json")
		if showJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(created); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(out, created.ID)
		}

		if attach, _ := cmd.Flags().GetBool("attach"); attach {
			return sessionAttachCmd.RunE(cmd, []string{created.ID})
		}
		return nil
	},
}

// sandboxListCmd is a thin alias over `session list`.
var sandboxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List running workers (sessions)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionListCmd.RunE(cmd, args)
	},
}

// sandboxStopCmd is a thin alias over `session close`.
var sandboxStopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop a worker by session id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionCloseCmd.RunE(cmd, args)
	},
}

// sandboxSendCmd is a literal alias of `session send` so messaging a worker by
// id is discoverable from the sandbox group; it shares the implementation to
// avoid divergence.
var sandboxSendCmd = &cobra.Command{
	Use:   "send <id> <message>",
	Short: "Send a message to a worker by session id",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sessionSendCmd.RunE(cmd, args)
	},
}

func init() {
	sandboxStartCmd.Flags().StringP("workspace", "w", "", "Workspace directory (default current dir)")
	sandboxStartCmd.Flags().Bool("json", false, "Output the created session as JSON")
	sandboxStartCmd.Flags().Bool("attach", false, "Stream the session's events after starting")
	sandboxStartCmd.Flags().String("idle", "", "Idle timeout for --attach (e.g. 30s)")
	sandboxListCmd.Flags().Bool("json", false, "Output as JSON")

	sandboxCmd.AddCommand(sandboxStartCmd, sandboxListCmd, sandboxStopCmd, sandboxSendCmd)
}
