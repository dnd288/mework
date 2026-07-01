// Package config holds the on-disk CLI/daemon configuration primitives that
// every mework component may depend on. It is the leaf config package with
// no dependency on cobra or other CLI frameworks.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultBaseURL is the default server URL when not overridden.
const DefaultBaseURL = "http://localhost:8080"

// DaemonConfig holds the agent-runtime settings persisted in config.json.
type DaemonConfig struct {
	// TriggerKeyword fires the agent when found in a ticket comment (default "/run").
	TriggerKeyword string `json:"trigger_keyword,omitempty"`
	// PollIntervalSeconds is the board poll cadence (default 5).
	PollIntervalSeconds int `json:"poll_interval_seconds,omitempty"`
	// WatchBoardIDs limits polling to specific boards; empty means all accessible boards.
	WatchBoardIDs []string `json:"watch_board_ids,omitempty"`
	// DoneColumnID, when set, moves a ticket here after the agent finishes.
	DoneColumnID string `json:"done_column_id,omitempty"`
	// Backends lists the AI CLIs to detect/use (e.g. claude, codex, opencode).
	Backends []string `json:"backends,omitempty"`
	// SandboxEngine selects the sandbox driver: "local" (default, host subprocess,
	// no isolation), "docker" (container per agent), or "cloudflare" (remote Workers AI).
	SandboxEngine string `json:"sandbox_engine,omitempty"`
}

// MezonCredentials holds the Mezon bot app credentials stored in config.
type MezonCredentials struct {
	AppID   string `json:"app_id,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// Config is the on-disk CLI/daemon configuration for a profile.
type Config struct {
	BaseURL      string           `json:"base_url,omitempty"`
	WorkspaceID  string           `json:"workspace_id,omitempty"`
	Token        string           `json:"token,omitempty"`
	ServerURL    string           `json:"server_url,omitempty"`
	RuntimeToken string           `json:"rt_token,omitempty"`
	Daemon       DaemonConfig     `json:"daemon,omitempty"`
	Mezon        *MezonCredentials `json:"mezon,omitempty"`
}

// LoadConfig reads the profile config from disk. A missing file yields a
// zero-value Config (not an error) so first-run commands work.
func LoadConfig(profile string) (*Config, error) {
	path := ConfigPath(profile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the config to the profile path with private permissions,
// creating the profile directory as needed.
func (c *Config) Save(profile string) error {
	if err := ensureDir(ProfileDir(profile)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// 0600: the file holds the bearer token.
	return os.WriteFile(ConfigPath(profile), data, 0o600)
}

// MeworkDir returns the root config directory (~/.mework), honoring MEWORK_HOME override.
func MeworkDir() string {
	if override := os.Getenv("MEWORK_HOME"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".mework"
	}
	return filepath.Join(home, ".mework")
}

// ProfileDir returns the directory holding config/state for the given profile.
func ProfileDir(profile string) string {
	if profile == "" {
		return MeworkDir()
	}
	return filepath.Join(MeworkDir(), "profiles", profile)
}

// ConfigPath is the JSON config file path for a profile.
func ConfigPath(profile string) string {
	return filepath.Join(ProfileDir(profile), "config.json")
}

// PidPath is the daemon pid file path for a profile.
func PidPath(profile string) string {
	return filepath.Join(ProfileDir(profile), "daemon.pid")
}

// LogPath is the daemon log file path for a profile.
func LogPath(profile string) string {
	return filepath.Join(ProfileDir(profile), "daemon.log")
}

// ensureDir creates a directory tree with private (0700) permissions.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
