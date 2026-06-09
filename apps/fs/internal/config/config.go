package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/utils"
)

type Config struct {
	Port     string
	ScanDirs []string

	// Hub connection
	HubWSURL  string // ws://hub:8080/ws/agent
	HubAPIURL string // http://hub:8080/api

	// Agent identity (persisted to DATA_DIR/agent.json)
	DataDir string

	// Registration (only needed for first-time setup)
	AgentToken string // one-time registration token from hub
	AgentLabel string // user-friendly label (e.g. "NAS Agent")
}

func Load() (*Config, error) {
	required := []string{
		"SCAN_DIRS",
		"HUB_WS_URL",
		"HUB_API_URL",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", key)
		}
	}

	cfg := &Config{
		Port:       getEnv("PORT", "8000"),
		ScanDirs:   ParseScanDirs(getEnv("SCAN_DIRS", "")),
		HubWSURL:   os.Getenv("HUB_WS_URL"),
		HubAPIURL:  os.Getenv("HUB_API_URL"),
		DataDir:    getEnv("DATA_DIR", "./data"),
		AgentToken: os.Getenv("AGENT_TOKEN"),
		AgentLabel: getEnv("AGENT_LABEL", ""),
	}

	if len(cfg.ScanDirs) == 0 {
		return nil, fmt.Errorf("got empty SCAN_DIRS, required at least one valid")
	}

	return cfg, nil
}

func ParseScanDirs(dirs string) []string {
	envDirs := strings.Split(dirs, ",")

	scanDirs := make([]string, 0)
	for _, dir := range envDirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed != "" && utils.IsValidDir(trimmed) {
			scanDirs = append(scanDirs, trimmed)
		}
	}

	return scanDirs
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
