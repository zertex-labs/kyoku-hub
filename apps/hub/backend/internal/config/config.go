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
