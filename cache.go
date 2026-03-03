package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// CachedData holds all data fetched from Semaphore UI
type CachedData struct {
	LastUpdated time.Time  `json:"last_updated"`
	Projects    []Project  `json:"projects"`
	Tasks       []Task     `json:"tasks"`
	Templates   []Template `json:"templates"`
	Schedules   []Schedule `json:"schedules"`
	Events      []Event    `json:"events"`
	Users       []User     `json:"users"`
}

// Cache provides thread-safe file-backed caching
type Cache struct {
	mu       sync.RWMutex
	filePath string
	data     *CachedData
}

// NewCache creates a Cache backed by the given file path
func NewCache(filePath string) *Cache {
	c := &Cache{filePath: filePath}
	// Try to load existing cache from disk
	if err := c.loadFromDisk(); err != nil {
		slog.Warn("Cache: no existing cache found, starting fresh", "error", err)
		c.data = &CachedData{}
	} else {
		slog.Info("Cache: loaded existing data", "path", filePath, "last_updated", c.data.LastUpdated.Format(time.RFC3339))
	}
	return c
}

// Get returns a copy of the current cached data
func (c *Cache) Get() *CachedData {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil {
		return &CachedData{}
	}
	// Return a shallow copy to avoid races on reads
	cp := *c.data
	return &cp
}

// Set stores new data in memory and persists it to disk
func (c *Cache) Set(data *CachedData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data.LastUpdated = time.Now()
	c.data = data
	if err := c.saveToDisk(data); err != nil {
		slog.Error("Cache: failed to persist to disk", "error", err)
	}
}

// Age returns how old the cached data is
func (c *Cache) Age() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil || c.data.LastUpdated.IsZero() {
		return 0
	}
	return time.Since(c.data.LastUpdated)
}

func (c *Cache) loadFromDisk() error {
	f, err := os.Open(c.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var data CachedData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return err
	}
	c.data = &data
	return nil
}

func (c *Cache) saveToDisk(data *CachedData) error {
	// Write to a temp file then rename for atomicity
	tmp := c.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()

	return os.Rename(tmp, c.filePath)
}
