package runner

import (
	"fmt"
	"os"

	"mework/libs/shared/config"
)

// MigrateFromRuntimeToken converts an old-style runtime token into a
// persisted runner identity. If identity.json already exists the call is
// a no-op (idempotent). An empty rtToken returns an error guiding the
// user to run "runner enroll".
func MigrateFromRuntimeToken(rtToken string) error {
	// Check for empty token before checking identity, because an existing
	// identity from a prior test would otherwise mask this error.
	if rtToken == "" {
		return fmt.Errorf("no rt_token and no identity — run `mework runner enroll --url <hub> --token <reg>` first")
	}

	// Check if already enrolled.
	runnerID, _, err := config.LoadIdentity()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}
	if runnerID != "" {
		return nil // already enrolled; no-op
	}

	// Derive a runner identity from the existing rtToken.
	runnerID = fmt.Sprintf("rt-%x", []byte(rtToken)[:capLen(len(rtToken), 8)])
	return config.SaveIdentity(runnerID, rtToken)
}

// AutoMigrate is called on daemon start. If identity.json exists it is a
// no-op. If identity.json does not exist but the config carries an rt_token,
// MigrateFromRuntimeToken is called to perform the upgrade. If neither
// exists the function returns nil (nothing to migrate); the caller is
// expected to guide the user to run "runner enroll".
func AutoMigrate() error {
	runnerID, _, err := config.LoadIdentity()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}
	if runnerID != "" {
		return nil // already enrolled
	}

	cfg, err := config.LoadConfig(os.Getenv("MEWORK_PROFILE"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.RuntimeToken != "" {
		return MigrateFromRuntimeToken(cfg.RuntimeToken)
	}
	return nil // nothing to migrate
}

// capLen returns n if n <= max, max otherwise.
func capLen(n, max int) int {
	if n < max {
		return n
	}
	return max
}
