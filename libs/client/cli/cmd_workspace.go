package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"mework/libs/client/catalog"
)

// workspacePackCmd zips a workspace directory into a catalog bundle file.
//
//	mework workspace pack <dir> [-o bundle.zip]
var workspacePackCmd = &cobra.Command{
	Use:   "pack <dir>",
	Short: "Pack a workspace directory into a catalog bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundle, err := catalog.Pack(args[0])
		if err != nil {
			return err
		}
		out, _ := cmd.Flags().GetString("output")
		if out == "" {
			out = "workspace-bundle.zip"
		}
		if err := os.WriteFile(out, bundle, 0o644); err != nil {
			return fmt.Errorf("write bundle %s: %w", out, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "packed %s -> %s (%d bytes)\n", args[0], out, len(bundle))
		return nil
	},
}

// workspacePushCmd packs a workspace directory and registers it as an immutable
// version of the named agent/workspace in the catalog.
//
//	mework workspace push <dir> <name> <version>
var workspacePushCmd = &cobra.Command{
	Use:   "push <dir> <name> <version>",
	Short: "Pack a workspace and publish it as a catalog version",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, name, version := args[0], args[1], args[2]
		bundle, err := catalog.Pack(dir)
		if err != nil {
			return err
		}
		baseURL, _ := cmd.Flags().GetString("server-url")
		if err := catalog.Push(cmd.Context(), baseURL, name, version, bundle); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "pushed %s as %s@%s\n", dir, name, version)
		return nil
	},
}

// workspacePullCmd fetches a registered bundle from a local file and extracts it
// into a destination directory, recreating the workspace.
//
//	mework workspace pull <bundle.zip> <dest-dir>
var workspacePullCmd = &cobra.Command{
	Use:   "pull <bundle> <dest-dir>",
	Short: "Extract a workspace bundle into a directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundlePath, dest := args[0], args[1]
		bundle, err := os.ReadFile(bundlePath)
		if err != nil {
			return fmt.Errorf("read bundle %s: %w", bundlePath, err)
		}
		if err := catalog.ExtractWorkspace(bundle, dest); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "pulled %s -> %s\n", bundlePath, dest)
		return nil
	},
}

func init() {
	workspacePackCmd.Flags().StringP("output", "o", "", "Output bundle path (default workspace-bundle.zip)")
	workspaceCmd.AddCommand(workspacePackCmd, workspacePushCmd, workspacePullCmd)
}
