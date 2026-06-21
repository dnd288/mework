package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time variables injected via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// Command group IDs for help organization.
const (
	groupCore       = "core"
	groupRuntime    = "runtime"
	groupAdditional = "additional"
)

var debugFlag bool

var rootCmd = &cobra.Command{
	Use:           "mework",
	Short:         "Mework CLI — kanban management + local agent runtime",
	Long:          "Work with Mello from the command line: manage boards/tickets and run the agent daemon that executes triggered tickets locally.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)\ngo: %s, os/arch: %s/%s",
		version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	rootCmd.SetVersionTemplate("mework {{.Version}}\n")

	rootCmd.PersistentFlags().String("server-url", "", "Mello API base URL (env: MELLO_BASE_URL)")
	rootCmd.PersistentFlags().String("workspace-id", "", "Workspace ID (env: MELLO_WORKSPACE_ID)")
	rootCmd.PersistentFlags().String("profile", "", "Config profile name — isolates config, daemon state, and logs")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print full error details on failure (env: MEWORK_DEBUG)")

	rootCmd.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core Commands:"},
		&cobra.Group{ID: groupRuntime, Title: "Runtime Commands:"},
		&cobra.Group{ID: groupAdditional, Title: "Additional Commands:"},
	)
	// Place built-in help/completion under the additional group so they don't
	// render a duplicate ungrouped "Additional Commands:" section.
	rootCmd.SetHelpCommandGroupID(groupAdditional)
	rootCmd.SetCompletionCommandGroupID(groupAdditional)

	registerCommands()
}

// profile returns the resolved --profile flag value (env MELLO_PROFILE fallback).
func profile() string {
	if f := rootCmd.PersistentFlags().Lookup("profile"); f != nil && f.Changed {
		return f.Value.String()
	}
	return os.Getenv("MEWORK_PROFILE")
}

// Execute is the main entry point for the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
