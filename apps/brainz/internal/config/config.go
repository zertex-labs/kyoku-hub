package config

import (
	"os"
	"strconv"
	"time"
)

type CacheTier struct {
	MinHits int
	TTL     time.Duration
}

type Config struct {
	Port     string
	DBUrl    string
	LogLevel string

	// Worker
	ForceUpdateThreshold int
	WorkerPollInterval   time.Duration

	// Cache
	CacheMaxSizeMB int64
	CacheHitWindow  time.Duration
	CacheTTLTiers  []CacheTier
}

func Load() *Config {
	cfg := &Config{
		Port:     getEnv("PORT", "8081"),
		DBUrl:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5433/brainz?sslmode=disable"),
		LogLevel: getEnv("LOG_LEVEL", "debug"),

		ForceUpdateThreshold: getEnvInt("FORCE_UPDATE_THRESHOLD", 100),
		WorkerPollInterval:   getEnvDuration("WORKER_POLL_INTERVAL", 5*time.Second),

		CacheMaxSizeMB: int64(getEnvInt("CACHE_MAX_SIZE_MB", 256)),
		CacheHitWindow:  getEnvDuration("CACHE_HIT_WINDOW", 1*time.Hour),
		CacheTTLTiers: []CacheTier{
			{MinHits: 50, TTL: 6 * time.Hour},
			{MinHits: 10, TTL: 2 * time.Hour},
			{MinHits: 1, TTL: 30 * time.Minute},
			{MinHits: 0, TTL: 10 * time.Minute},
		},
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
