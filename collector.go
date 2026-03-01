package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector implements prometheus.Collector and wraps Semaphore data
type Collector struct {
	cfg    *Config
	client *SemaphoreClient
	cache  *Cache

	// Descriptors
	upDesc            *prometheus.Desc
	cacheAgeDesc      *prometheus.Desc
	lastScrapeDesc    *prometheus.Desc

	// Projects
	projectInfoDesc   *prometheus.Desc
	projectMaxParDesc *prometheus.Desc

	// Tasks
	taskInfoDesc      *prometheus.Desc
	taskDurationDesc  *prometheus.Desc
	taskStatusDesc    *prometheus.Desc

	// Events
	eventInfoDesc     *prometheus.Desc

	// Users
	userInfoDesc      *prometheus.Desc
	userCountDesc     *prometheus.Desc
}

// NewCollector creates a new Prometheus collector
func NewCollector(cfg *Config, client *SemaphoreClient, cache *Cache) *Collector {
	const ns = "semaphore"

	return &Collector{
		cfg:    cfg,
		client: client,
		cache:  cache,

		upDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "", "up"),
			"Whether the last scrape of Semaphore UI was successful (1 = yes, 0 = no)",
			nil, nil,
		),
		cacheAgeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "cache", "age_seconds"),
			"Age of the cached data in seconds",
			nil, nil,
		),
		lastScrapeDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "cache", "last_update_timestamp_seconds"),
			"Unix timestamp of the last successful cache update",
			nil, nil,
		),

		// Projects
		projectInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "project", "info"),
			"Semaphore project metadata (value is always 1)",
			[]string{"project_id", "project_name", "alert_chat", "created"}, nil,
		),
		projectMaxParDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "project", "max_parallel_tasks"),
			"Maximum number of parallel tasks allowed for the project",
			[]string{"project_id", "project_name"}, nil,
		),

		// Tasks
		taskInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "task", "info"),
			"Semaphore task metadata (value is always 1)",
			[]string{
				"task_id", "project_id", "template_id",
				"status", "playbook", "message",
				"debug", "dry_run", "diff",
				"created",
			}, nil,
		),
		taskDurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "task", "duration_seconds"),
			"Duration of a completed task in seconds (-1 if still running or no end time)",
			[]string{"task_id", "project_id", "template_id", "status"}, nil,
		),
		taskStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "task", "status_total"),
			"Number of tasks per project/status combination",
			[]string{"project_id", "status"}, nil,
		),

		// Events
		eventInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "event", "info"),
			"Semaphore audit event (value is always 1)",
			[]string{
				"object_type", "object_id",
				"project_id", "description",
				"user_id", "user_name", "username",
				"created",
			}, nil,
		),

		// Users
		userInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "user", "info"),
			"Semaphore user metadata (value is always 1)",
			[]string{"user_id", "name", "username", "email", "admin", "external"}, nil,
		),
		userCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "user", "count"),
			"Total number of Semaphore users",
			nil, nil,
		),
	}
}

// Register registers the collector with the default Prometheus registry
func (c *Collector) Register() error {
	return prometheus.Register(c)
}

// Describe implements prometheus.Collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.upDesc
	ch <- c.cacheAgeDesc
	ch <- c.lastScrapeDesc
	ch <- c.projectInfoDesc
	ch <- c.projectMaxParDesc
	ch <- c.taskInfoDesc
	ch <- c.taskDurationDesc
	ch <- c.taskStatusDesc
	ch <- c.eventInfoDesc
	ch <- c.userInfoDesc
	ch <- c.userCountDesc
}

// Collect implements prometheus.Collector — reads from cache, never calls API
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	data := c.cache.Get()

	// Determine if data is fresh enough
	up := 1.0
	if data.LastUpdated.IsZero() {
		up = 0.0
	}

	ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, up)
	ch <- prometheus.MustNewConstMetric(c.cacheAgeDesc, prometheus.GaugeValue, c.cache.Age().Seconds())

	if !data.LastUpdated.IsZero() {
		ch <- prometheus.MustNewConstMetric(c.lastScrapeDesc, prometheus.GaugeValue,
			float64(data.LastUpdated.Unix()))
	}

	// Projects
	for _, p := range data.Projects {
		ch <- prometheus.MustNewConstMetric(
			c.projectInfoDesc, prometheus.GaugeValue, 1,
			strconv.Itoa(p.ID), p.Name, p.AlertChat, p.Created,
		)
		ch <- prometheus.MustNewConstMetric(
			c.projectMaxParDesc, prometheus.GaugeValue, float64(p.MaxParallel),
			strconv.Itoa(p.ID), p.Name,
		)
	}

	// Tasks — aggregate counts per project+status
	statusCount := make(map[string]map[string]int) // project_id -> status -> count
	for _, t := range data.Tasks {
		pid := strconv.Itoa(t.ProjectID)
		if statusCount[pid] == nil {
			statusCount[pid] = make(map[string]int)
		}
		statusCount[pid][t.Status]++

		// Task info
		ch <- prometheus.MustNewConstMetric(
			c.taskInfoDesc, prometheus.GaugeValue, 1,
			strconv.Itoa(t.ID),
			strconv.Itoa(t.ProjectID),
			strconv.Itoa(t.TemplateID),
			t.Status,
			t.Playbook,
			t.Message,
			strconv.FormatBool(t.Debug),
			strconv.FormatBool(t.DryRun),
			strconv.FormatBool(t.Diff),
			t.Created.Format(time.RFC3339),
		)

		// Task duration
		dur := -1.0
		if t.Start != nil && t.End != nil {
			dur = t.End.Sub(*t.Start).Seconds()
		}
		ch <- prometheus.MustNewConstMetric(
			c.taskDurationDesc, prometheus.GaugeValue, dur,
			strconv.Itoa(t.ID),
			strconv.Itoa(t.ProjectID),
			strconv.Itoa(t.TemplateID),
			t.Status,
		)
	}
	for pid, statuses := range statusCount {
		for status, count := range statuses {
			ch <- prometheus.MustNewConstMetric(
				c.taskStatusDesc, prometheus.GaugeValue, float64(count),
				pid, status,
			)
		}
	}

	// Events
	for _, e := range data.Events {
		objectID := ""
		if e.ObjectID != nil {
			objectID = strconv.Itoa(*e.ObjectID)
		}
		projectID := ""
		if e.ProjectID != nil {
			projectID = strconv.Itoa(*e.ProjectID)
		}
		userID := ""
		if e.UserID != nil {
			userID = strconv.Itoa(*e.UserID)
		}

		ch <- prometheus.MustNewConstMetric(
			c.eventInfoDesc, prometheus.GaugeValue, 1,
			e.ObjectType,
			objectID,
			projectID,
			e.Description,
			userID,
			e.UserName,
			e.Username,
			e.Created.Format(time.RFC3339),
		)
	}

	// Users
	for _, u := range data.Users {
		ch <- prometheus.MustNewConstMetric(
			c.userInfoDesc, prometheus.GaugeValue, 1,
			strconv.Itoa(u.ID), u.Name, u.Username, u.Email,
			strconv.FormatBool(u.Admin),
			strconv.FormatBool(u.External),
		)
	}
	ch <- prometheus.MustNewConstMetric(
		c.userCountDesc, prometheus.GaugeValue, float64(len(data.Users)),
	)
}

// FetchAndCache calls the Semaphore API and stores results in the cache
func (c *Collector) FetchAndCache() error {
	data := &CachedData{}

	// Projects
	projects, err := c.client.GetProjects()
	if err != nil {
		return fmt.Errorf("fetching projects: %w", err)
	}
	data.Projects = projects
	log.Printf("Fetched %d projects", len(projects))

	// Tasks and Templates per project
	for _, p := range projects {
		tasks, err := c.client.GetTasks(p.ID)
		if err != nil {
			log.Printf("Warning: failed to fetch tasks for project %d (%s): %v", p.ID, p.Name, err)
			continue
		}
		data.Tasks = append(data.Tasks, tasks...)

		templates, err := c.client.GetTemplates(p.ID)
		if err != nil {
			log.Printf("Warning: failed to fetch templates for project %d (%s): %v", p.ID, p.Name, err)
		} else {
			data.Templates = append(data.Templates, templates...)
		}
	}
	log.Printf("Fetched %d tasks, %d templates", len(data.Tasks), len(data.Templates))

	// Events
	events, err := c.client.GetEvents(c.cfg.MaxEvents)
	if err != nil {
		log.Printf("Warning: failed to fetch events: %v", err)
	} else {
		data.Events = events
		log.Printf("Fetched %d events", len(events))
	}

	// Users (may fail if not admin)
	users, err := c.client.GetUsers()
	if err != nil {
		log.Printf("Warning: failed to fetch users (requires admin): %v", err)
	} else {
		data.Users = users
		log.Printf("Fetched %d users", len(users))
	}

	c.cache.Set(data)
	return nil
}
