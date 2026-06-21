package server

import "mework/server/hub"

// LoadConfig loads the server configuration from environment variables.
func LoadConfig() (*hub.Config, error) {
	return hub.LoadConfig()
}
