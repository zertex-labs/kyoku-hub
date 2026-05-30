package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zertex-labs/kyoku-hub/apps/fs/internal/utils"
)

type Config struct {
		Port  string
		ScanDirs []string
}

func Load() (*Config, error) {
	required := []string{
		"SCAN_DIRS",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", key)
		}
	}

	cfg := &Config{
		Port: getEnv("PORT", "8000"),
		ScanDirs: ParseScanDirs(getEnv("SCAN_DIRS", "")),
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

