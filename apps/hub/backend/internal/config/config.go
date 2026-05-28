package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	MusicBrainzClientID     string
	MusicBrainzClientSecret string
	BaseURL                 string // Backend base URL (for OAuth redirect)
	FrontendURL             string // Frontend URL (for CORS + post-login redirect)
	SessionSecret           string
	BrainzWSURL             string // Brainz WS endpoint URL
	BrainzAPIURL            string // Brainz HTTP API base URL
	InstanceID              string // This hub's unique instance ID
	SecureCookies           bool   // Whether to set Secure flag on cookies (derived from BaseURL)
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		MusicBrainzClientID:     os.Getenv("MUSICBRAINZ_CLIENT_ID"),
		MusicBrainzClientSecret: os.Getenv("MUSICBRAINZ_CLIENT_SECRET"),
		BaseURL:                 getEnv("BASE_URL", "http://localhost:8080"),
		FrontendURL:             getEnv("FRONTEND_URL", "http://localhost:3000"),
		SessionSecret:           os.Getenv("SESSION_SECRET"),
		BrainzWSURL:             getEnv("BRAINZ_WS_URL", "ws://localhost:8081/ws"),
		BrainzAPIURL:            getEnv("BRAINZ_API_URL", "http://localhost:8081/api"),
		InstanceID:              getEnv("INSTANCE_ID", "hub-dev-01"),
	}

	cfg.SecureCookies = strings.HasPrefix(cfg.BaseURL, "https://")

	required := []string{
		"DATABASE_URL",
		"MUSICBRAINZ_CLIENT_ID",
		"MUSICBRAINZ_CLIENT_SECRET",
		"SESSION_SECRET",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", key)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
