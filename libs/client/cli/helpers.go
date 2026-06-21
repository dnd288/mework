package cli

import (
	"os"

	"github.com/spf13/cobra"

	"mework/libs/shared/config"
)

// FlagOrEnv resolves a string value with precedence: flag > env var > fallback.
// A flag is considered set only if explicitly Changed by the user, so an
// unset flag does not clobber an env var.
func FlagOrEnv(cmd *cobra.Command, flagName, envName, fallback string) string {
	if cmd != nil {
		if f := cmd.Flags().Lookup(flagName); f != nil && f.Changed {
			return f.Value.String()
		}
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return fallback
}

// ResolveBaseURL applies flag > env (MELLO_BASE_URL) > config > DefaultBaseURL.
func ResolveBaseURL(cmd *cobra.Command, cfg *config.Config) string {
	cfgVal := ""
	if cfg != nil {
		cfgVal = cfg.BaseURL
	}
	if cfgVal == "" {
		cfgVal = config.DefaultBaseURL
	}
	return FlagOrEnv(cmd, "server-url", "MELLO_BASE_URL", cfgVal)
}

// ResolveWorkspaceID applies flag > env (MELLO_WORKSPACE_ID) > config.
func ResolveWorkspaceID(cmd *cobra.Command, cfg *config.Config) string {
	cfgVal := ""
	if cfg != nil {
		cfgVal = cfg.WorkspaceID
	}
	return FlagOrEnv(cmd, "workspace-id", "MELLO_WORKSPACE_ID", cfgVal)
}

// ResolveToken applies env (MELLO_API_KEY) > config. No flag: tokens must not
// be passed as flags (they leak into shell history / process listings).
func ResolveToken(cfg *config.Config) string {
	if v := os.Getenv("MELLO_API_KEY"); v != "" {
		return v
	}
	if cfg != nil {
		return cfg.Token
	}
	return ""
}
