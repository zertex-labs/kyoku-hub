package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/zertex-labs/kyoku-hub/packages/validation"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	MusicBrainzClientID     string
	MusicBrainzClientSecret string
	BaseURL                 string // Backend base URL (for OAuth redirect)
	FrontendURL             string // Frontend URL (for CORS + post-login redirect)
	SessionSecret           string
	BrainzWSURL             string   // Brainz WS endpoint URL
	BrainzAPIURL            string   // Brainz HTTP API base URL
	InstanceID              string   // This hub's unique instance ID
	SecureCookies           bool     // Whether to set Secure flag on cookies (derived from BaseURL)
	RequiredTags            []string // Tags an fs track must have to be considered complete

	// Tidal downloader (all optional; download endpoints return 503 if unset)
	TidalToken        string
	TidalRefreshToken string
	TidalCountryCode  string
	TidalUserID       string
	TidalExpiresAt    int64  // Unix timestamp
	DownloadDir       string // Directory where downloaded albums are written
	PluginsDir        string // User-installed post-download plugins
}

// defaultPluginsDir is ~/.config/kyoku/plugins, so a plugin is installed by
// dropping a directory there and nothing else.
func defaultPluginsDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "./plugins"
	}
	return dir + "/kyoku/plugins"
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

		TidalToken:        os.Getenv("TIDAL_TOKEN"),
		TidalRefreshToken: os.Getenv("TIDAL_REFRESH_TOKEN"),
		TidalCountryCode:  getEnv("TIDAL_COUNTRY_CODE", "US"),
		TidalUserID:       os.Getenv("TIDAL_USER_ID"),
		DownloadDir:       getEnv("DOWNLOAD_DIR", "./downloads"),
		PluginsDir:        getEnv("PLUGINS_DIR", defaultPluginsDir()),
	}

	if exp := os.Getenv("TIDAL_EXPIRES_AT"); exp != "" {
		cfg.TidalExpiresAt, _ = strconv.ParseInt(exp, 10, 64)
	}

	cfg.SecureCookies = strings.HasPrefix(cfg.BaseURL, "https://")

	requiredTags, err := validation.ParseRequiredTags(os.Getenv("REQUIRED_TAGS"))
	if err != nil {
		return nil, fmt.Errorf("invalid REQUIRED_TAGS: %w", err)
	}
	cfg.RequiredTags = requiredTags

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
