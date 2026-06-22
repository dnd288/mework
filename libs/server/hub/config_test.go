package hub

import (
	"strings"
	"testing"
)

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
