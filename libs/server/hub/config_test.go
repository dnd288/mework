package hub

import (
	"strings"
	"testing"
)

func TestLoadConfig_ChannelRoutingDefaultOff(t *testing.T) {
	const okKey = "0123456789abcdef"
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("SERVER_KEY", okKey)
	t.Setenv("MEWORK_SECRET_KEY", okKey)

	// Unset → disabled by default.
	t.Setenv("CHANNEL_ROUTING_ENABLED", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ChannelRoutingEnabled {
		t.Error("channel routing must be OFF by default")
	}

	// Explicit truthy → enabled.
	t.Setenv("CHANNEL_ROUTING_ENABLED", "true")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.ChannelRoutingEnabled {
		t.Error("CHANNEL_ROUTING_ENABLED=true must enable channel routing")
	}
}

func TestLoadConfig_KeyStrength(t *testing.T) {
	const okKey = "0123456789abcdef" // 16 chars

	cases := []struct {
		name      string
		serverKey string
		secretKey string
		wantErr   string // substring; "" means success
	}{
		{"valid", okKey, okKey, ""},
		{"short server key", "short", okKey, "SERVER_KEY must be at least"},
		{"short secret key", okKey, "short", "MEWORK_SECRET_KEY must be at least"},
		{"empty server key", "", okKey, "SERVER_KEY is required"},
		{"empty secret key", okKey, "", "MEWORK_SECRET_KEY is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/db")
			t.Setenv("SERVER_KEY", tc.serverKey)
			t.Setenv("MEWORK_SECRET_KEY", tc.secretKey)

			_, err := LoadConfig()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadConfig: unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("LoadConfig error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
