package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// --- Semaphore API models ---

// Project represents a Semaphore project
type Project struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Alert       bool   `json:"alert"`
	AlertChat   string `json:"alert_chat"`
	MaxParallel int    `json:"max_parallel_tasks"`
	Created     string `json:"created"`
}

// Task represents a Semaphore task/job run
type Task struct {
	ID            int        `json:"id"`
	TemplateID    int        `json:"template_id"`
	Status        string     `json:"status"`
	Debug         bool       `json:"debug"`
	DryRun        bool       `json:"dry_run"`
	Diff          bool       `json:"diff"`
	Playbook      string     `json:"playbook"`
	Environment   string     `json:"environment"`
	UserID        *int       `json:"user_id"`
	ProjectID     int        `json:"project_id"`
	Version       *string    `json:"version"`
	Message       string     `json:"message"`
	CommitHash    *string    `json:"commit_hash"`
	CommitMessage string     `json:"commit_message"`
	Start         *time.Time `json:"start"`
	End           *time.Time `json:"end"`
	Created       time.Time  `json:"created"`
}

// Template represents a task template.
// SurveyVars is json.RawMessage because Semaphore returns either a JSON
// array or null — not a plain string.
type Template struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	ProjectID   int             `json:"project_id"`
	Playbook    string          `json:"playbook"`
	Description string          `json:"description"`
	Type        string          `json:"type"`
	SurveyVars  json.RawMessage `json:"survey_vars"`
}

// Schedule represents a cron-based task schedule attached to a template
type Schedule struct {
	ID            int    `json:"id"`
	ProjectID     int    `json:"project_id"`
	TemplateID    int    `json:"template_id"`
	CronFormat    string `json:"cron_format"`
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	DeleteAfterRun bool  `json:"delete_after_run"`
}

// Event represents a Semaphore event/audit log entry
type Event struct {
	ProjectID   *int      `json:"project_id"`
	ObjectID    *int      `json:"object_id"`
	ObjectType  string    `json:"object_type"`
	Description string    `json:"description"`
	Created     time.Time `json:"created"`
	UserID      *int      `json:"user_id"`
	UserName    string    `json:"user_name"`
	Username    string    `json:"username"`
}

// User represents a Semaphore user
type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Admin    bool   `json:"admin"`
	External bool   `json:"external"`
	Alert    bool   `json:"alert"`
}

// --- Client ---

// SemaphoreClient handles API communication with Semaphore UI
type SemaphoreClient struct {
	cfg        *Config
	httpClient *http.Client
}

// NewSemaphoreClient creates a new API client
func NewSemaphoreClient(cfg *Config) *SemaphoreClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		},
	}
	return &SemaphoreClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout:   cfg.HTTPTimeout,
			Transport: transport,
		},
	}
}

func (c *SemaphoreClient) get(path string, target interface{}) error {
	url := fmt.Sprintf("%s/api%s", c.cfg.SemaphoreURL, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return nil
}

// GetProjects fetches all projects
func (c *SemaphoreClient) GetProjects() ([]Project, error) {
	var projects []Project
	err := c.get("/projects", &projects)
	return projects, err
}

// GetTasks fetches recent tasks for a project
func (c *SemaphoreClient) GetTasks(projectID int) ([]Task, error) {
	var tasks []Task
	err := c.get(fmt.Sprintf("/project/%d/tasks", projectID), &tasks)
	return tasks, err
}

// GetTemplates fetches templates for a project
func (c *SemaphoreClient) GetTemplates(projectID int) ([]Template, error) {
	var templates []Template
	err := c.get(fmt.Sprintf("/project/%d/templates", projectID), &templates)
	return templates, err
}

// GetSchedules fetches schedules for a project
func (c *SemaphoreClient) GetSchedules(projectID int) ([]Schedule, error) {
	var schedules []Schedule
	err := c.get(fmt.Sprintf("/project/%d/schedules", projectID), &schedules)
	return schedules, err
}

// GetEvents fetches events and truncates to the configured limit.
// The Semaphore API does not reliably honour the ?limit query parameter,
// so we apply the limit client-side after receiving the full response.
func (c *SemaphoreClient) GetEvents(limit int) ([]Event, error) {
	var events []Event
	if err := c.get("/events", &events); err != nil {
		return nil, err
	}
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

// GetUsers fetches all users (admin only)
func (c *SemaphoreClient) GetUsers() ([]User, error) {
	var users []User
	err := c.get("/users", &users)
	return users, err
}

// GetUser fetches the current authenticated user
func (c *SemaphoreClient) GetUser() (*User, error) {
	var user User
	err := c.get("/user", &user)
	return &user, err
}
