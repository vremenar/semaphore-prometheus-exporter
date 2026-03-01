package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	// Semaphore connection
	SemaphoreURL string
	APIToken     string

	// HTTP server
	ListenAddress string

	// Scraping
	ScrapeInterval time.Duration
	MaxEvents      int

	// Cache
	CacheFile string

	// Timeouts
	HTTPTimeout time.Duration

	// TLS
	InsecureSkipVerify bool
}

// LoadConfig reads configuration from environment variables with sensible defaults.
func LoadConfig() *Config {
	cfg := &Config{
		SemaphoreURL:       getEnv("SEMAPHORE_URL", "http://localhost:3000"),
		APIToken:           getEnvRequired("SEMAPHORE_API_TOKEN"),
		ListenAddress:      getEnv("LISTEN_ADDRESS", ":9090"),
		ScrapeInterval:     getDuration("SCRAPE_INTERVAL", 30*time.Minute),
		MaxEvents:          getInt("MAX_EVENTS", 100),
		CacheFile:          getEnv("CACHE_FILE", "/opt/semaphore-prometheus-exporter/data/cache.json"),
		HTTPTimeout:        getDuration("HTTP_TIMEOUT", 30*time.Second),
		InsecureSkipVerify: getBool("INSECURE_SKIP_VERIFY", false),
	}

	// Ensure cache directory exists
	dir := filepath.Dir(cfg.CacheFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Error("Failed to create cache directory", "path", dir, "error", err)
		os.Exit(1)
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("Required environment variable is not set", "key", key)
		os.Exit(1)
	}
	return v
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("Invalid integer value, using default", "key", key, "value", v, "default", fallback)
			return fallback
		}
		return n
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Warn("Invalid duration value, using default", "key", key, "value", v, "default", fallback)
			return fallback
		}
		return d
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			slog.Warn("Invalid bool value, using default", "key", key, "value", v, "default", fallback)
			return fallback
		}
		return b
	}
	return fallback
}
