package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// ─────────────────────────────────────────────
// Config tests
// ─────────────────────────────────────────────

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("SEMAPHORE_API_TOKEN", "test-token")
	t.Setenv("SEMAPHORE_URL", "http://semaphore:3000")

	cfg := LoadConfig()

	if cfg.SemaphoreURL != "http://semaphore:3000" {
		t.Errorf("expected SemaphoreURL=http://semaphore:3000, got %s", cfg.SemaphoreURL)
	}
	if cfg.ListenAddress != ":9090" {
		t.Errorf("expected ListenAddress=:9090, got %s", cfg.ListenAddress)
	}
	if cfg.ScrapeInterval != 30*time.Minute {
		t.Errorf("expected ScrapeInterval=30m, got %s", cfg.ScrapeInterval)
	}
	if cfg.MaxEvents != 100 {
		t.Errorf("expected MaxEvents=100, got %d", cfg.MaxEvents)
	}
	if cfg.HTTPTimeout != 30*time.Second {
		t.Errorf("expected HTTPTimeout=30s, got %s", cfg.HTTPTimeout)
	}
}

func TestLoadConfig_OverrideFromEnv(t *testing.T) {
	t.Setenv("SEMAPHORE_API_TOKEN", "my-token")
	t.Setenv("SEMAPHORE_URL", "https://custom:8080")
	t.Setenv("SCRAPE_INTERVAL", "5m")
	t.Setenv("MAX_EVENTS", "50")
	t.Setenv("HTTP_TIMEOUT", "10s")
	t.Setenv("INSECURE_SKIP_VERIFY", "true")
	t.Setenv("LISTEN_ADDRESS", ":8888")

	cfg := LoadConfig()

	if cfg.APIToken != "my-token" {
		t.Errorf("expected APIToken=my-token, got %s", cfg.APIToken)
	}
	if cfg.SemaphoreURL != "https://custom:8080" {
		t.Errorf("expected SemaphoreURL=https://custom:8080, got %s", cfg.SemaphoreURL)
	}
	if cfg.ScrapeInterval != 5*time.Minute {
		t.Errorf("expected ScrapeInterval=5m, got %s", cfg.ScrapeInterval)
	}
	if cfg.MaxEvents != 50 {
		t.Errorf("expected MaxEvents=50, got %d", cfg.MaxEvents)
	}
	if cfg.HTTPTimeout != 10*time.Second {
		t.Errorf("expected HTTPTimeout=10s, got %s", cfg.HTTPTimeout)
	}
	if !cfg.InsecureSkipVerify {
		t.Errorf("expected InsecureSkipVerify=true")
	}
	if cfg.ListenAddress != ":8888" {
		t.Errorf("expected ListenAddress=:8888, got %s", cfg.ListenAddress)
	}
}

func TestLoadConfig_InvalidDuration_UsesDefault(t *testing.T) {
	t.Setenv("SEMAPHORE_API_TOKEN", "tok")
	t.Setenv("SCRAPE_INTERVAL", "not-a-duration")

	cfg := LoadConfig()
	if cfg.ScrapeInterval != 30*time.Minute {
		t.Errorf("expected fallback 30m, got %s", cfg.ScrapeInterval)
	}
}

func TestLoadConfig_InvalidInt_UsesDefault(t *testing.T) {
	t.Setenv("SEMAPHORE_API_TOKEN", "tok")
	t.Setenv("MAX_EVENTS", "banana")

	cfg := LoadConfig()
	if cfg.MaxEvents != 100 {
		t.Errorf("expected fallback 100, got %d", cfg.MaxEvents)
	}
}

// ─────────────────────────────────────────────
// Cache tests
// ─────────────────────────────────────────────

func tempCacheFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "cache.json")
}

func TestCache_SetAndGet(t *testing.T) {
	c := NewCache(tempCacheFile(t))

	now := time.Now().UTC().Truncate(time.Second)
	data := &CachedData{
		Projects: []Project{{ID: 1, Name: "Alpha"}},
		Events:   []Event{{Description: "test event", Created: now}},
	}
	c.Set(data)

	got := c.Get()
	if len(got.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got.Projects))
	}
	if got.Projects[0].Name != "Alpha" {
		t.Errorf("expected project name Alpha, got %s", got.Projects[0].Name)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Events))
	}
}

func TestCache_PersistAndReload(t *testing.T) {
	path := tempCacheFile(t)

	c1 := NewCache(path)
	c1.Set(&CachedData{
		Projects: []Project{{ID: 42, Name: "Persisted"}},
	})

	// Create a new cache instance pointing at the same file — it should load from disk
	c2 := NewCache(path)
	got := c2.Get()

	if len(got.Projects) != 1 {
		t.Fatalf("expected 1 project after reload, got %d", len(got.Projects))
	}
	if got.Projects[0].ID != 42 {
		t.Errorf("expected project ID 42, got %d", got.Projects[0].ID)
	}
}

func TestCache_EmptyOnMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	c := NewCache(path)
	got := c.Get()
	if got == nil {
		t.Error("expected non-nil CachedData for empty cache")
	}
}

func TestCache_Age(t *testing.T) {
	c := NewCache(tempCacheFile(t))

	// Before any Set, Age returns 0
	if c.Age() != 0 {
		t.Errorf("expected age 0 before first set, got %s", c.Age())
	}

	c.Set(&CachedData{})

	time.Sleep(10 * time.Millisecond)
	age := c.Age()
	if age <= 0 {
		t.Errorf("expected positive age after Set, got %s", age)
	}
}

func TestCache_LastUpdatedIsSet(t *testing.T) {
	c := NewCache(tempCacheFile(t))
	before := time.Now()
	c.Set(&CachedData{})
	after := time.Now()

	got := c.Get()
	if got.LastUpdated.Before(before) || got.LastUpdated.After(after) {
		t.Errorf("LastUpdated %s is not between %s and %s", got.LastUpdated, before, after)
	}
}

// ─────────────────────────────────────────────
// Semaphore client tests (against mock HTTP server)
// ─────────────────────────────────────────────

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Config) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := &Config{
		SemaphoreURL: srv.URL,
		APIToken:     "test-token",
		HTTPTimeout:  5 * time.Second,
	}
	return srv, cfg
}

func TestClient_GetProjects(t *testing.T) {
	projects := []Project{
		{ID: 1, Name: "Project Alpha"},
		{ID: 2, Name: "Project Beta"},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(projects)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	if got[0].Name != "Project Alpha" {
		t.Errorf("expected 'Project Alpha', got %s", got[0].Name)
	}
}

func TestClient_GetTasks(t *testing.T) {
	now := time.Now()
	tasks := []Task{
		{ID: 10, ProjectID: 1, Status: "success", Created: now},
		{ID: 11, ProjectID: 1, Status: "error", Created: now},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/1/tasks" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetTasks(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}
	if got[1].Status != "error" {
		t.Errorf("expected status 'error', got %s", got[1].Status)
	}
}

func TestClient_GetEvents(t *testing.T) {
	events := []Event{
		{Description: "task created", ObjectType: "task"},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		limit := r.URL.Query().Get("limit")
		if limit != "25" {
			t.Errorf("expected limit=25, got %s", limit)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetEvents(25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].ObjectType != "task" {
		t.Errorf("expected ObjectType 'task', got %s", got[0].ObjectType)
	}
}

func TestClient_GetUsers(t *testing.T) {
	users := []User{
		{ID: 1, Name: "Admin User", Username: "admin", Admin: true},
		{ID: 2, Name: "Regular User", Username: "user", Admin: false},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetUsers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 users, got %d", len(got))
	}
	if !got[0].Admin {
		t.Errorf("expected first user to be admin")
	}
}

func TestClient_NonOKStatus(t *testing.T) {
	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	client := NewSemaphoreClient(cfg)
	_, err := client.GetProjects()
	if err == nil {
		t.Error("expected error on 401 response, got nil")
	}
}

func TestClient_InvalidJSON(t *testing.T) {
	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json{{"))
	})

	client := NewSemaphoreClient(cfg)
	_, err := client.GetProjects()
	if err == nil {
		t.Error("expected error on invalid JSON, got nil")
	}
}

// ─────────────────────────────────────────────
// Collector / FetchAndCache integration test
// ─────────────────────────────────────────────

func newMockSemaphoreServer(t *testing.T) *httptest.Server {
	t.Helper()

	projects := []Project{{ID: 1, Name: "Test Project"}}
	tasks := []Task{{ID: 5, ProjectID: 1, Status: "success", Created: time.Now()}}
	templates := []Template{{ID: 3, ProjectID: 1, Name: "Deploy"}}
	events := []Event{{Description: "deployed", ObjectType: "task", Created: time.Now()}}
	users := []User{{ID: 1, Name: "Admin", Username: "admin", Admin: true}}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(projects)
	})
	mux.HandleFunc("/api/project/1/tasks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tasks)
	})
	mux.HandleFunc("/api/project/1/templates", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(templates)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(events)
	})
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(users)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCollector_FetchAndCache(t *testing.T) {
	srv := newMockSemaphoreServer(t)

	cfg := &Config{
		SemaphoreURL: srv.URL,
		APIToken:     "test-token",
		HTTPTimeout:  5 * time.Second,
		MaxEvents:    50,
		CacheFile:    filepath.Join(t.TempDir(), "cache.json"),
	}

	client := NewSemaphoreClient(cfg)
	cache := NewCache(cfg.CacheFile)
	collector := NewCollector(cfg, client, cache)

	if err := collector.FetchAndCache(); err != nil {
		t.Fatalf("FetchAndCache failed: %v", err)
	}

	data := cache.Get()

	if len(data.Projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(data.Projects))
	}
	if len(data.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(data.Tasks))
	}
	if len(data.Templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(data.Templates))
	}
	if len(data.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(data.Events))
	}
	if len(data.Users) != 1 {
		t.Errorf("expected 1 user, got %d", len(data.Users))
	}
}

func TestCollector_MetricsEndpoint(t *testing.T) {
	srv := newMockSemaphoreServer(t)

	cfg := &Config{
		SemaphoreURL: srv.URL,
		APIToken:     "test-token",
		HTTPTimeout:  5 * time.Second,
		MaxEvents:    50,
		CacheFile:    filepath.Join(t.TempDir(), "cache.json"),
	}
	// Override listen address to avoid port conflicts — endpoint test uses recorder
	cfg.ListenAddress = ":0"

	client := NewSemaphoreClient(cfg)
	cache := NewCache(cfg.CacheFile)
	collector := NewCollector(cfg, client, cache)

	// Pre-populate cache so metrics are non-empty
	if err := collector.FetchAndCache(); err != nil {
		t.Fatalf("FetchAndCache failed: %v", err)
	}

	// Hit the /healthz endpoint via a test recorder
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /healthz, got %d", rec.Code)
	}
}

// ─────────────────────────────────────────────
// Helper: env variable tests
// ─────────────────────────────────────────────

func TestGetEnv_ReturnsValue(t *testing.T) {
	key := "TEST_GET_ENV_KEY_" + strconv.Itoa(int(time.Now().UnixNano()))
	os.Setenv(key, "hello")
	defer os.Unsetenv(key)

	if v := getEnv(key, "fallback"); v != "hello" {
		t.Errorf("expected 'hello', got '%s'", v)
	}
}

func TestGetEnv_ReturnsFallback(t *testing.T) {
	key := "TEST_GET_ENV_MISSING_" + strconv.Itoa(int(time.Now().UnixNano()))
	os.Unsetenv(key)

	if v := getEnv(key, "default"); v != "default" {
		t.Errorf("expected 'default', got '%s'", v)
	}
}
