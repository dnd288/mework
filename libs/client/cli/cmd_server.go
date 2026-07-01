package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// serverCmd is the parent grouping command for in-process hub operations.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the mework hub in-process (start)",
}

// serverStartCmd runs the hub in-process within the mework binary. The actual
// wiring (config load, migrations, pool, HTTP server with graceful shutdown)
// lives behind the SetServerStarter seam so the libs/client module stays free
// of any libs/server import.
//
// DisableFlagParsing keeps --listen out of cobra's persistent pflag.Changed
// state, which otherwise leaks across sequential Execute()/RunE calls (the same
// idiom used by `runner enroll`).
var serverStartCmd = &cobra.Command{
	Use:                "start",
	Short:              "Start the hub in-process (reads config from the environment)",
	Long:               "Start the hub in-process: load config from the environment (DATABASE_URL, SERVER_KEY, MEWORK_SECRET_KEY, optional LISTEN_ADDR/MELLO_BASE_URL/…), run migrations, and serve the hub with graceful shutdown. Suitable as the command of a docker-compose service alongside a Postgres service.",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Manual flag parse. An empty listen means "no override" — the starter
		// then honors the configured/default address (LISTEN_ADDR or :8080).
		listen := ""
		for i := 0; i < len(args); i++ {
			if args[i] == "--listen" && i+1 < len(args) {
				listen = args[i+1]
				i++
			}
		}

		if serverStartFn == nil {
			return fmt.Errorf("server start is not available in this build")
		}
		return serverStartFn(cmd.Context(), listen)
	},
}

func init() {
	// Register the flag so it appears in help text.
	serverStartCmd.Flags().String("listen", ":8080", "Listen address override (env: LISTEN_ADDR)")
	serverCmd.AddCommand(serverStartCmd)
}
