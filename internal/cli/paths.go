package cli

import (
	"os"
	"path/filepath"
)

// MeworkDir returns the root config directory (~/.mework), honoring MEWORK_HOME override.
func MeworkDir() string {
	if override := os.Getenv("MEWORK_HOME"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to current dir if home is unavailable; better than panicking.
		return ".mework"
	}
	return filepath.Join(home, ".mework")
}

// ProfileDir returns the directory holding config/state for the given profile.
// The empty profile maps to the root MeworkDir so the default case stays flat.
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

// StatePath is the daemon trigger-state cache path for a profile.
func StatePath(profile string) string {
	return filepath.Join(ProfileDir(profile), "state.json")
}

// ensureDir creates a directory tree with private (0700) permissions.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
