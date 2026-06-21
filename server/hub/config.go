package hub

import (
	"errors"
	"os"

	"mework/server/bus"
)

// Config holds the environment configuration for the mework server.
type Config struct {
	DatabaseURL     string
	ListenAddr      string
	WebhookSecret   string
	ServerKey       string
	MeworkSecretKey string
	MelloBaseURL    string

	// Broker is an optional pre-configured message bus broker. When nil,
	// NewServer creates a default in-memory broker. Tests that need to
	// share a broker between the server and a client harness set this
	// to the same broker instance.
	Broker bus.Broker
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

	meworkSecretKey := os.Getenv("MEWORK_SECRET_KEY")
	if meworkSecretKey == "" {
		return nil, errors.New("MEWORK_SECRET_KEY is required but not set")
	}

	melloBaseURL := os.Getenv("MELLO_BASE_URL")
	if melloBaseURL == "" {
		melloBaseURL = "https://mello.mezon.vn/api/v1"
	}

	return &Config{
		DatabaseURL:     dbURL,
		ListenAddr:      listenAddr,
		WebhookSecret:   webhookSecret,
		ServerKey:       serverKey,
		MeworkSecretKey: meworkSecretKey,
		MelloBaseURL:    melloBaseURL,
	}, nil
}
