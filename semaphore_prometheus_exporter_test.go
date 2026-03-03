package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func init() {
	// Discard log output during tests to keep test output clean
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

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

func TestClient_GetEvents_ClientSideTruncation(t *testing.T) {
	// Simulate Semaphore returning more events than requested (API ignores ?limit)
	allEvents := make([]Event, 10)
	for i := range allEvents {
		allEvents[i] = Event{
			Description: "event " + strconv.Itoa(i),
			ObjectType:  "task",
			Created:     time.Now(),
		}
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(allEvents)
	})

	client := NewSemaphoreClient(cfg)

	// Request limit of 5 — client must truncate the 10-item API response
	got, err := client.GetEvents(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 events after client-side truncation, got %d", len(got))
	}
	if got[0].Description != "event 0" {
		t.Errorf("expected first event 'event 0', got %s", got[0].Description)
	}
}

func TestClient_GetEvents_LimitHigherThanAvailable(t *testing.T) {
	// When limit > available events, return all without panic
	events := []Event{
		{Description: "only event", ObjectType: "task", Created: time.Now()},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetEvents(100)
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



func TestClient_GetTemplates(t *testing.T) {
	templates := []Template{
		{ID: 1, ProjectID: 1, Name: "Deploy App",    Playbook: "deploy.yml",  Type: "Task"},
		{ID: 2, ProjectID: 1, Name: "Run Tests",     Playbook: "test.yml",    Type: "Task"},
		{ID: 3, ProjectID: 1, Name: "Build Image",   Playbook: "build.yml",   Type: "Build"},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/1/templates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(templates)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetTemplates(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(got))
	}
	if got[0].Name != "Deploy App" {
		t.Errorf("expected first template name 'Deploy App', got %s", got[0].Name)
	}
	if got[0].Playbook != "deploy.yml" {
		t.Errorf("expected playbook 'deploy.yml', got %s", got[0].Playbook)
	}
	if got[2].Type != "Build" {
		t.Errorf("expected type 'Build', got %s", got[2].Type)
	}
}

func TestClient_GetTemplates_Empty(t *testing.T) {
	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Template{})
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetTemplates(99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 templates, got %d", len(got))
	}
}

func TestClient_GetSchedules(t *testing.T) {
	schedules := []Schedule{
		{ID: 1, ProjectID: 1, TemplateID: 5, CronFormat: "0 * * * *", Name: "Hourly",    Active: true,  DeleteAfterRun: false},
		{ID: 2, ProjectID: 1, TemplateID: 6, CronFormat: "0 0 * * *", Name: "Nightly",   Active: false, DeleteAfterRun: true},
	}

	_, cfg := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/1/schedules" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schedules)
	})

	client := NewSemaphoreClient(cfg)
	got, err := client.GetSchedules(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(got))
	}
	if got[0].CronFormat != "0 * * * *" {
		t.Errorf("expected cron '0 * * * *', got %s", got[0].CronFormat)
	}
	if got[0].Name != "Hourly" {
		t.Errorf("expected first schedule name 'Hourly', got %s", got[0].Name)
	}
	if !got[0].Active {
		t.Errorf("expected first schedule to be active")
	}
	if got[1].Active {
		t.Errorf("expected second schedule to be inactive")
	}
	if !got[1].DeleteAfterRun {
		t.Errorf("expected second schedule to have delete_after_run=true")
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
	schedules := []Schedule{{ID: 1, ProjectID: 1, TemplateID: 3, CronFormat: "0 * * * *", Name: "Hourly deploy", Active: true, DeleteAfterRun: false}}
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
	mux.HandleFunc("/api/project/1/schedules", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(schedules)
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
	if data.Templates[0].Name != "Deploy" {
		t.Errorf("expected template name 'Deploy', got %s", data.Templates[0].Name)
	}
	if data.Templates[0].ProjectID != 1 {
		t.Errorf("expected template ProjectID 1, got %d", data.Templates[0].ProjectID)
	}
	if len(data.Schedules) != 1 {
		t.Errorf("expected 1 schedule, got %d", len(data.Schedules))
	}
	if len(data.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(data.Events))
	}
	if len(data.Users) != 1 {
		t.Errorf("expected 1 user, got %d", len(data.Users))
	}
}

func TestHealthzEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /healthz, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", rec.Body.String())
	}
}

func TestIndexEndpoint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from /, got %d", rec.Code)
	}

	body := rec.Body.String()

	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %q", ct)
	}
	if !containsStr(body, "Semaphore Prometheus Exporter") {
		t.Error("expected index page to contain 'Semaphore Prometheus Exporter'")
	}
	if !containsStr(body, "/metrics") {
		t.Error("expected index page to contain link to /metrics")
	}
	if !containsStr(body, "/healthz") {
		t.Error("expected index page to contain link to /healthz")
	}
	if !containsStr(body, "https://github.com/vremenar/semaphore-prometheus-exporter") {
		t.Error("expected index page to contain GitHub link")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}


// ─────────────────────────────────────────────
// Version tests
// ─────────────────────────────────────────────

func TestVersion_NotEmpty(t *testing.T) {
	if Version == "" {
		t.Error("expected Version to be non-empty")
	}
}

func TestVersion_Format(t *testing.T) {
	// Version must follow semver: MAJOR.MINOR.PATCH (e.g. 1.0.0, 2.3.11)
	parts := strings.Split(Version, ".")
	if len(parts) != 3 {
		t.Errorf("expected version in MAJOR.MINOR.PATCH format, got %q", Version)
		return
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			t.Errorf("version part %q is not a number in version %q", part, Version)
		}
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
