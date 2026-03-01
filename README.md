# Semaphore Prometheus Exporter

A lightweight Prometheus exporter for [Semaphore UI](https://semaphoreui.com/) written in Go.

It polls the Semaphore REST API on a configurable interval, stores the data in a
local file-backed cache, and exposes all metrics via a standard `/metrics` endpoint
— so that every Prometheus scrape reads from cache and never hammers the API.

---

## Exposed Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `semaphore_up` | Gauge | 1 if data has been fetched at least once, 0 otherwise |
| `semaphore_cache_age_seconds` | Gauge | Age of the cached data in seconds |
| `semaphore_cache_last_update_timestamp_seconds` | Gauge | Unix timestamp of the last successful update |
| `semaphore_project_info` | Gauge | Project metadata (labels: id, name, alert_chat, created) |
| `semaphore_project_max_parallel_tasks` | Gauge | Max parallel tasks per project |
| `semaphore_task_info` | Gauge | Task metadata (labels: id, project, template, status, playbook, …) |
| `semaphore_task_duration_seconds` | Gauge | Task wall-clock duration (-1 if no end time) |
| `semaphore_task_status_total` | Gauge | Task count per project and status |
| `semaphore_event_info` | Gauge | Audit event metadata (last N events, configurable) |
| `semaphore_user_info` | Gauge | User metadata (labels: id, name, username, email, admin) |
| `semaphore_user_count` | Gauge | Total number of users |

---

## Quick Start

### 1. Clone and configure

```bash
cp .env.example .env
# Edit .env and set at minimum SEMAPHORE_URL and SEMAPHORE_API_TOKEN
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
| `SEMAPHORE_API_TOKEN` | *(required)* | API token from Semaphore UI → User Settings → API Tokens |
| `LISTEN_ADDRESS` | `:9090` | Address the HTTP server binds to |
| `SCRAPE_INTERVAL` | `30m` | How often to fetch from Semaphore (Go duration: `30s`, `5m`, `1h`) |
| `MAX_EVENTS` | `100` | Number of audit events to fetch and expose |
| `HTTP_TIMEOUT` | `30s` | Timeout for HTTP requests to Semaphore |
| `INSECURE_SKIP_VERIFY` | `false` | Skip TLS certificate verification (not recommended in production) |
| `CACHE_FILE` | `/opt/semaphore-exporter/data/cache.json` | Path of the JSON cache file inside the container |
| `CACHE_DATA_PATH` | `./data` | **Docker Compose only** — host path mounted as the cache volume |
| `EXPORTER_PORT` | `9090` | **Docker Compose only** — host port the metrics endpoint is exposed on |

---

## Generating an API Token

1. Log in to Semaphore UI
2. Click your username → **Your Profile**
3. Scroll to **API Tokens** → **Add Token**
4. Copy the token and set it as `SEMAPHORE_API_TOKEN`

> **Note:** Fetching users requires an admin account. If a non-admin token is used,
> user metrics will be empty but all other metrics will still work.

---

## Prometheus Scrape Config

```yaml
scrape_configs:
  - job_name: semaphore
    static_configs:
      - targets: ["semaphore-exporter:9090"]
    scrape_interval: 1m   # Can be faster than SCRAPE_INTERVAL — reads from cache
```

---

## Building Manually

```bash
go mod download
go build -o semaphore-exporter .
./semaphore-exporter
```

---

## Volume / Persistence

The cache directory `/opt/semaphore-exporter/data` is declared as a Docker volume.
On restart the exporter will load the last-known data from disk immediately and
serve it until the first successful API fetch completes.

To use a custom host path, set `CACHE_DATA_PATH` in your `.env` file:

```dotenv
CACHE_DATA_PATH=/var/lib/semaphore-exporter
```

---

## Health Check

```
GET /healthz  → 200 OK  (always, as long as the process is alive)
GET /         → HTML index page with links
```
