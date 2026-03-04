# 📡 Semaphore Prometheus Exporter

A lightweight Prometheus exporter for [Semaphore UI](https://semaphoreui.com/) written in Go.

It polls the Semaphore REST API on a configurable interval, stores the data in a
local file-backed cache, and exposes all metrics via a standard `/metrics` endpoint
— so that every Prometheus scrape reads from cache and never hammers the API.

[![CI/CD Pipeline](https://github.com/vremenar/semaphore-prometheus-exporter/actions/workflows/docker-image-ci.yml/badge.svg)](https://github.com/vremenar/semaphore-prometheus-exporter/actions/workflows/docker-image-ci.yml)

---

## Exposed Metrics

### Exporter Health

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_up` | — | `1` if data has been fetched at least once, `0` otherwise |
| `semaphore_cache_age_seconds` | — | Age of the cached data in seconds |
| `semaphore_cache_last_update_timestamp_seconds` | — | Unix timestamp of the last successful cache update |

### Projects

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_project_info` | `project_id`, `project_name`, `alert_chat`, `created` | Project metadata (value is always 1) |
| `semaphore_project_max_parallel_tasks` | `project_id`, `project_name` | Maximum parallel tasks allowed per project |

### Tasks

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_task_info` | `task_id`, `project_id`, `template_id`, `status`, `playbook`, `message`, `debug`, `dry_run`, `diff`, `created` | Task metadata (value is always 1) |
| `semaphore_task_duration_seconds` | `task_id`, `project_id`, `template_id`, `status` | Task wall-clock duration in seconds (`-1` if still running or no end time recorded) |
| `semaphore_task_status_total` | `project_id`, `status` | Task count per project and status combination |

### Templates

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_template_info` | `template_id`, `project_id`, `name`, `playbook`, `description`, `type` | Template metadata (value is always 1) |
| `semaphore_template_count` | `project_id`, `project_name` | Total number of templates per project |

### Schedules

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_schedule_info` | `schedule_id`, `project_id`, `template_id`, `cron_format`, `name`, `active`, `delete_after_run` | Schedule metadata (value is always 1) |
| `semaphore_schedule_count` | `project_id`, `project_name` | Total number of schedules per project |

### Events

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_event_info` | `object_type`, `object_id`, `project_id`, `description`, `user_id`, `user_name`, `username`, `created` | Audit event metadata — last N events (configurable via `MAX_EVENTS`) |

### Users

| Metric | Labels | Description |
|--------|--------|-------------|
| `semaphore_user_info` | `user_id`, `name`, `username`, `email`, `admin`, `external` | User metadata (value is always 1) |
| `semaphore_user_count` | — | Total number of users |

> **Note:** Fetching users requires an admin API token. If a non-admin token is used,
> user metrics will be empty but all other metrics will still work.

---

## Quick Start

### 1. Clone and configure

```bash
cp .env.example .env
# Edit .env — set at minimum SEMAPHORE_URL and SEMAPHORE_API_TOKEN
```

### 2. Run with Docker Compose

```bash
docker compose up -d
```

Metrics are available at **http://localhost:9090/metrics**.

---

## Configuration Reference

All settings are controlled via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SEMAPHORE_URL` | *(required)* | Base URL of your Semaphore instance, e.g. `http://semaphore:3000` |
| `SEMAPHORE_API_TOKEN` | *(required)* | API token — Semaphore UI → Your Profile → API Tokens |
| `LISTEN_ADDRESS` | `:9090` | Address the HTTP server binds to |
| `SCRAPE_INTERVAL` | `30m` | How often to fetch from Semaphore (Go duration: `30s`, `5m`, `1h`) |
| `MAX_EVENTS` | `100` | Number of audit events to fetch and expose |
| `HTTP_TIMEOUT` | `30s` | Timeout for HTTP requests to Semaphore |
| `INSECURE_SKIP_VERIFY` | `false` | Skip TLS certificate verification (not recommended in production) |
| `CACHE_FILE` | `/opt/semaphore-prometheus-exporter/data/cache.json` | Path of the JSON cache file inside the container |
| `CACHE_DATA_PATH` | `./data` | **Docker Compose only** — host path mounted as the cache volume |
| `EXPORTER_PORT` | `9090` | **Docker Compose only** — host port the metrics endpoint is exposed on |
| `LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |

---

## Generating an API Token

1. Log in to Semaphore UI
2. Click your username → **Your Profile**
3. Scroll to **API Tokens** → **Add Token**
4. Copy the token and set it as `SEMAPHORE_API_TOKEN`

---

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: semaphore
    static_configs:
      - targets: ["semaphore-prometheus-exporter:9090"]
    scrape_interval: 1m   # Can be faster than SCRAPE_INTERVAL — reads from cache
```

---

## Grafana Dashboard

A ready-to-import Grafana dashboard is included at `grafana-dashboard.json`.

**Import steps:**
1. Grafana → **Dashboards** → **Import**
2. Upload `grafana-dashboard.json`
3. Select your Prometheus datasource
4. Click **Import**

The dashboard covers:
- Exporter health, cache age and last update time
- Task status distribution (pie chart) and counts per project
- Last 3 failed tasks and last 3 successful tasks
- Task duration trends, average and maximum
- Project list with max parallel tasks
- Audit event breakdown by object type and user
- Full user list with admin/external indicators

---

## Logging

All log output is in **JSON format** following the [Elastic Common Schema (ECS) 1.12](https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html), making it compatible with Wazuh, Elasticsearch, and other ECS-aware SIEM systems.

Example log entry:

```json
{
  "@timestamp": "2026-03-01T19:23:26.123456789Z",
  "log": { "level": "info" },
  "message": "Fetched events",
  "count": 100,
  "service": { "name": "semaphore-prometheus-exporter", "type": "metrics" },
  "ecs": { "version": "1.12.0" }
}
```

Log level is controlled via the `LOG_LEVEL` environment variable (`debug`, `info`, `warn`, `error`).

---

## Building Manually

```bash
go mod download
go build -ldflags="-X main.Version=1.0.0" -o semaphore-prometheus-exporter .
./semaphore-prometheus-exporter
```

---

## Versioning

The application version is defined in [`version.go`](version.go):

```go
const Version = "1.0.0"
```

Update this value manually before each release. The CI/CD pipeline reads it automatically and applies it as a Docker image tag alongside `latest` and the current date:

```
docker.io/vremenar/semaphore-prometheus-exporter:latest
docker.io/vremenar/semaphore-prometheus-exporter:2026-03-01
docker.io/vremenar/semaphore-prometheus-exporter:1.0.0
```

---

## Volume / Persistence

The cache directory `/opt/semaphore-prometheus-exporter/data` is declared as a Docker volume.
On restart the exporter loads the last-known data from disk immediately and serves it
until the first successful API fetch completes.

To use a custom host path, set `CACHE_DATA_PATH` in your `.env` file:

```dotenv
CACHE_DATA_PATH=/var/lib/semaphore-prometheus-exporter
```

---

## Health Check

```
GET /healthz  → 200 OK  (always, as long as the process is alive)
GET /         → HTML index page with links to /metrics, /healthz and GitHub
```