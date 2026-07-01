// Package mezon holds shared Mezon provider types used by both bot and provider
// code, without pulling in the heavy SDK dependency.
package mezon

// Config holds Mezon bot credentials for a connection.
type Config struct {
	AppID   string `json:"app_id"`
	APIKey  string `json:"api_key"` // sealed at rest
	BaseURL string `json:"base_url,omitempty"`
}

const (
	DefaultBaseURL = "https://api.mezon.vn"
	DefaultWSURL   = "wss://gateway.mezon.vn"
)
