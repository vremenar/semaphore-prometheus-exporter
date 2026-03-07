package main

import (
	"embed"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:embed static/index.html
var staticFiles embed.FS

func main() {
	setupLogger()

	cfg := LoadConfig()

	slog.Info("Starting Semaphore Prometheus Exporter",
		"version", Version,
		"semaphore_url", cfg.SemaphoreURL,
		"listen_address", cfg.ListenAddress,
		"scrape_interval", cfg.ScrapeInterval.String(),
		"cache_file", cfg.CacheFile,
		"max_events", cfg.MaxEvents,
	)

	client := NewSemaphoreClient(cfg)
	cache := NewCache(cfg.CacheFile)
	collector := NewCollector(cfg, client, cache)

	if err := collector.Register(); err != nil {
		slog.Error("Failed to register collector", "error", err)
		os.Exit(1)
	}

	slog.Info("Performing initial data fetch")
	if err := collector.FetchAndCache(); err != nil {
		slog.Warn("Initial fetch failed", "error", err)
	}

	go func() {
		ticker := time.NewTicker(cfg.ScrapeInterval)
		defer ticker.Stop()
		for range ticker.C {
			slog.Info("Fetching data from Semaphore UI")
			if err := collector.FetchAndCache(); err != nil {
				slog.Error("Error fetching data", "error", err)
			} else {
				slog.Info("Data fetched and cached successfully")
			}
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", healthzHandler(cfg, client, cache))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			slog.Error("Failed to read index.html", "error", err)
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	slog.Info("Listening", "address", cfg.ListenAddress)
	if err := http.ListenAndServe(cfg.ListenAddress, mux); err != nil {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
}
