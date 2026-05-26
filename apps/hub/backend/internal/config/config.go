package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	MusicBrainzClientID    string
	MusicBrainzClientSecret string
	BaseURL                string // Backend base URL (for OAuth redirect)
	FrontendURL            string // Frontend URL (for CORS + post-login redirect)
	SessionSecret          string
	BrainzWSURL            string // Brainz WS endpoint URL
	BrainzAPIURL           string // Brainz HTTP API base URL
	InstanceID             string // This hub's unique instance ID
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            getEnv("DATABASE_URL", "postgres://kyoku:kyoku@localhost:5432/kyoku_hub?sslmode=disable"),
		MusicBrainzClientID:    os.Getenv("MUSICBRAINZ_CLIENT_ID"),
		MusicBrainzClientSecret: os.Getenv("MUSICBRAINZ_CLIENT_SECRET"),
		BaseURL:                getEnv("BASE_URL", "http://localhost:8080"),
		FrontendURL:            getEnv("FRONTEND_URL", "http://localhost:3000"),
		SessionSecret:          getEnv("SESSION_SECRET", "change-me-in-production-please"),
		BrainzWSURL:            getEnv("BRAINZ_WS_URL", "ws://localhost:8081/ws"),
		BrainzAPIURL:           getEnv("BRAINZ_API_URL", "http://localhost:8081/api"),
		InstanceID:             getEnv("INSTANCE_ID", "hub-dev-01"),
	}

	if cfg.MusicBrainzClientID == "" {
		return nil, fmt.Errorf("MUSICBRAINZ_CLIENT_ID is required")
	}
	if cfg.MusicBrainzClientSecret == "" {
		return nil, fmt.Errorf("MUSICBRAINZ_CLIENT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
