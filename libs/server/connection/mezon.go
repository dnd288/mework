package connection

// MezonConnectionConfig holds the provider-agnostic config for a Mezon connection.
type MezonConnectionConfig struct {
	MezonAppID string `json:"mezon_app_id,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

// ExtractMezonConfig extracts Mezon-specific config from a connection's Config map.
func ExtractMezonConfig(cfg map[string]any) MezonConnectionConfig {
	return MezonConnectionConfig{
		MezonAppID: getString(cfg, "mezon_app_id"),
		BaseURL:    getString(cfg, "base_url"),
	}
}

// IsMezonConnection returns true if the connection is for the Mezon provider.
func IsMezonConnection(conn *Connection) bool {
	return conn != nil && conn.ProviderCode == "mezon"
}

// getString is a helper to safely extract a string value from a config map.
func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
