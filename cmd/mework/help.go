package main

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"mework/internal/cli"
)

// registerCommands wires all top-level commands into the root. Command groups
// from later phases attach here; phase 01 ships only `config` + version.
func registerCommands() {
	configCmd.GroupID = groupAdditional
	rootCmd.AddCommand(configCmd)

	daemonCmd.GroupID = groupRuntime
	rootCmd.AddCommand(daemonCmd)

	loginCmd.GroupID = groupAdditional
	authCmd.GroupID = groupAdditional
	rootCmd.AddCommand(loginCmd, authCmd)

	for _, c := range []*cobra.Command{workspaceCmd, boardCmd, ticketCmd, commentCmd, searchCmd} {
		c.GroupID = groupCore
		rootCmd.AddCommand(c)
	}

	versionCmd.GroupID = groupAdditional
	rootCmd.AddCommand(versionCmd)
}

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Show or modify CLI configuration",
	GroupID: groupAdditional,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved configuration (token masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := cli.LoadConfig(profile())
		if err != nil {
			return err
		}
		// Mask the token before display.
		masked := *cfg
		masked.Token = maskToken(cfg.Token)
		out, err := json.MarshalIndent(masked, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd, configSetCmd)
}

// configKeys is the whitelist of settable config keys.
var configKeys = map[string]func(*cli.Config, string){
	"base_url":               func(c *cli.Config, v string) { c.BaseURL = v },
	"workspace_id":           func(c *cli.Config, v string) { c.WorkspaceID = v },
	"mcp_url":                func(c *cli.Config, v string) { c.MCPURL = v },
	"daemon.trigger_keyword": func(c *cli.Config, v string) { c.Daemon.TriggerKeyword = v },
	"daemon.done_column_id":  func(c *cli.Config, v string) { c.Daemon.DoneColumnID = v },
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value (keys: base_url, workspace_id, mcp_url, daemon.trigger_keyword, daemon.done_column_id)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		apply, ok := configKeys[key]
		if !ok {
			return fmt.Errorf("unknown config key %q", key)
		}
		if key == "mcp_url" || key == "base_url" {
			if u, err := url.ParseRequestURI(value); err != nil || u.Scheme == "" {
				return fmt.Errorf("%s must be a valid URL", key)
			}
		}
		cfg, err := cli.LoadConfig(profile())
		if err != nil {
			return err
		}
		apply(cfg, value)
		if err := cfg.Save(profile()); err != nil {
			return err
		}
		fmt.Printf("Set %s\n", key)
		return nil
	},
}

// maskToken hides all but the last 4 chars of a secret for display.
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}
