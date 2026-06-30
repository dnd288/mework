package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BotConfig defines a single Mezon bot the worker should connect.
type BotConfig struct {
	AppID   string `json:"app_id"`
	APIKey  string `json:"api_key"`
	Name    string `json:"name,omitempty"`
	Plan    string `json:"plan,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// Config holds all configuration for the mework-mezon-worker binary.
type Config struct {
	Bots            []BotConfig
	MeworkServerURL string
	MeworkToken     string
	PollInterval    time.Duration
	CursorDir       string

	// Redis URL for turbo engine state (optional, in-memory fallback when empty).
	RedisURL string
}

// Load reads configuration from environment variables and/or config file.
// Required: at least one bot configured, MEWORK_TOKEN.
func Load() (*Config, error) {
	cfg := &Config{
		MeworkServerURL: env("MEWORK_SERVER_URL", "http://localhost:8080"),
		MeworkToken:     os.Getenv("MEWORK_TOKEN"),
		PollInterval:    durationEnv("POLL_INTERVAL", 5*time.Second),
		CursorDir:       env("CURSOR_DIR", "./.mework-mezon-cursor"),
		RedisURL:        os.Getenv("REDIS_URL"),
	}

	// Load bots from config file (MEZON_CONFIG) or env vars.
	if configFile := os.Getenv("MEZON_CONFIG"); configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("read mezon config: %w", err)
		}
		var fileCfg struct {
			Bots []BotConfig `json:"bots"`
		}
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("parse mezon config: %w", err)
		}
		cfg.Bots = fileCfg.Bots
	}

	// Fall back to single bot from env vars (backward compat).
	if len(cfg.Bots) == 0 {
		if appID := os.Getenv("MEZON_APP_ID"); appID != "" {
			cfg.Bots = []BotConfig{{
				AppID:   appID,
				APIKey:  os.Getenv("MEZON_API_KEY"),
				Name:    env("MEZON_BOT_NAME", appID),
				Plan:    env("MEZON_PLAN", "pro"),
				BaseURL: os.Getenv("MEZON_BASE_URL"),
			}}
		}
	}

	// Validate.
	if len(cfg.Bots) == 0 {
		return nil, fmt.Errorf("no bots configured — set MEZON_APP_ID+MEZON_API_KEY, or MEZON_CONFIG")
	}
	if cfg.MeworkToken == "" {
		return nil, fmt.Errorf("MEWORK_TOKEN is required")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
