package server

import (
	"errors"
	"os"
)

// Config holds the environment configuration for the mework server.
type Config struct {
	DatabaseURL   string
	ListenAddr    string
	WebhookSecret string
	ServerKey     string
}

// LoadConfig loads the configuration from environment variables.
func LoadConfig() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is required but not set")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080" // Default port
	}

	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	// For now we don't enforce webhook secret to be set in development,
	// but it will be required in production. Let's make it optional but log/flag it later.

	serverKey := os.Getenv("SERVER_KEY")
	if serverKey == "" {
		return nil, errors.New("SERVER_KEY is required but not set")
	}

	return &Config{
		DatabaseURL:   dbURL,
		ListenAddr:    listenAddr,
		WebhookSecret: webhookSecret,
		ServerKey:     serverKey,
	}, nil
}
