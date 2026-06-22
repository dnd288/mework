package hub

import (
	"errors"
	"fmt"
	"os"

	"mework/libs/server/bus"
	"mework/libs/server/storage"
)

// minKeyLen is the minimum length for the server HMAC key and the secret-sealing
// key, so a trivially-weak (e.g. 1-character) key is rejected at startup rather
// than silently accepted and stretched by SHA-256.
const minKeyLen = 16

// Config holds the environment configuration for the mework server.
type Config struct {
	DatabaseURL     string
	ListenAddr      string
	WebhookSecret   string
	ServerKey       string
	MeworkSecretKey string
	MelloBaseURL    string

	// Storage configures the object storage backend.
	Storage storage.Config

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

	serverKey := os.Getenv("SERVER_KEY")
	if serverKey == "" {
		return nil, errors.New("SERVER_KEY is required but not set")
	}
	if len(serverKey) < minKeyLen {
		return nil, fmt.Errorf("SERVER_KEY must be at least %d characters", minKeyLen)
	}

	meworkSecretKey := os.Getenv("MEWORK_SECRET_KEY")
	if meworkSecretKey == "" {
		return nil, errors.New("MEWORK_SECRET_KEY is required but not set")
	}
	if len(meworkSecretKey) < minKeyLen {
		return nil, fmt.Errorf("MEWORK_SECRET_KEY must be at least %d characters", minKeyLen)
	}

	melloBaseURL := os.Getenv("MELLO_BASE_URL")
	if melloBaseURL == "" {
		melloBaseURL = "https://mello.mezon.vn/api/v1"
	}

	// Storage config from environment.
	storageCfg := storage.Config{
		Driver: storage.DriverName(os.Getenv("STORAGE_DRIVER")),
		Endpoint:   os.Getenv("STORAGE_ENDPOINT"),
		Region:     os.Getenv("STORAGE_REGION"),
		Bucket:     os.Getenv("STORAGE_BUCKET"),
		BasePath:   os.Getenv("STORAGE_BASE_PATH"),
	}
	storageCfg.Credentials.AccessKey = os.Getenv("STORAGE_ACCESS_KEY")
	storageCfg.Credentials.SecretKey = os.Getenv("STORAGE_SECRET_KEY")

	// Default to fs driver when no driver is specified.
	if storageCfg.Driver == "" {
		storageCfg.Driver = storage.DriverFS
	}
	if storageCfg.Bucket == "" {
		storageCfg.Bucket = "mework"
	}

	return &Config{
		DatabaseURL:     dbURL,
		ListenAddr:      listenAddr,
		WebhookSecret:   webhookSecret,
		ServerKey:       serverKey,
		MeworkSecretKey: meworkSecretKey,
		MelloBaseURL:    melloBaseURL,
		Storage:         storageCfg,
	}, nil
}
