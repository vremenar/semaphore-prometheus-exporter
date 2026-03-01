package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := LoadConfig()

	log.Printf("Starting Semaphore Prometheus Exporter")
	log.Printf("Semaphore URL: %s", cfg.SemaphoreURL)
	log.Printf("Listen address: %s", cfg.ListenAddress)
	log.Printf("Scrape interval: %s", cfg.ScrapeInterval)
	log.Printf("Cache file: %s", cfg.CacheFile)
	log.Printf("Max events: %d", cfg.MaxEvents)

	client := NewSemaphoreClient(cfg)
	cache := NewCache(cfg.CacheFile)
	collector := NewCollector(cfg, client, cache)

	// Register Prometheus collector
	if err := collector.Register(); err != nil {
		log.Fatalf("Failed to register collector: %v", err)
	}

	// Initial fetch
	log.Println("Performing initial data fetch...")
	if err := collector.FetchAndCache(); err != nil {
		log.Printf("Warning: initial fetch failed: %v", err)
	}

	// Start background scraper
	go func() {
		ticker := time.NewTicker(cfg.ScrapeInterval)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("Fetching data from Semaphore UI...")
			if err := collector.FetchAndCache(); err != nil {
				log.Printf("Error fetching data: %v", err)
			} else {
				log.Println("Data fetched and cached successfully")
			}
		}
	}()

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>
<head><title>Semaphore Prometheus Exporter</title></head>
<body>
<h1>Semaphore Prometheus Exporter</h1>
<p><a href="/metrics">Metrics</a></p>
<p><a href="/healthz">Health</a></p>
</body>
</html>`))
	})

	log.Printf("Listening on %s", cfg.ListenAddress)
	if err := http.ListenAndServe(cfg.ListenAddress, mux); err != nil {
		log.Fatalf("HTTP server error: %v", err)
		os.Exit(1)
	}
}
