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

// ResolveBaseURL applies flag > env (MEWORK_SERVER_URL / MELLO_BASE_URL) > config > DefaultBaseURL.
func ResolveBaseURL(cmd *cobra.Command, cfg *config.Config) string {
	cfgVal := ""
	if cfg != nil {
		cfgVal = cfg.ServerURL
	}
	if cfgVal == "" {
		cfgVal = cfg.BaseURL
	}
	if cfgVal == "" {
		cfgVal = config.DefaultBaseURL
	}
	return FlagOrEnv(cmd, "server-url", "MELLO_BASE_URL", cfgVal)
}

// ResolveWorkspaceID applies flag > env (MEWORK_WORKSPACE_ID / MELLO_WORKSPACE_ID) > config.
func ResolveWorkspaceID(cmd *cobra.Command, cfg *config.Config) string {
	cfgVal := ""
	if cfg != nil {
		cfgVal = cfg.WorkspaceID
	}
	return FlagOrEnv(cmd, "workspace-id", "MEWORK_WORKSPACE_ID", FlagOrEnv(cmd, "workspace-id", "MELLO_WORKSPACE_ID", cfgVal))
}

// ResolveToken applies env (MEWORK_API_KEY or MELLO_API_KEY) > config.
// MEWORK_API_KEY is the provider-neutral alternative; MELLO_API_KEY is
// retained for backward compatibility. No flag: tokens must not be passed
// as flags (they leak into shell history / process listings).
func ResolveToken(cfg *config.Config) string {
	if v := os.Getenv("MEWORK_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("MELLO_API_KEY"); v != "" {
		return v
	}
	if cfg != nil {
		return cfg.Token
	}
	return ""
}
