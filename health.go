package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// healthStatus represents the overall health check response
type healthStatus struct {
	Status    string                    `json:"status"`
	Version   string                    `json:"version"`
	Timestamp string                    `json:"timestamp"`
	Checks    map[string]*healthCheck   `json:"checks"`
}

// healthCheck represents a single health check result
type healthCheck struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func statusOK() *healthCheck    { return &healthCheck{Status: "ok"} }
func statusFail(msg string) *healthCheck { return &healthCheck{Status: "fail", Message: msg} }

// healthzHandler performs live checks and returns a structured JSON response.
// Returns HTTP 200 if all checks pass, HTTP 503 if any check fails.
func healthzHandler(cfg *Config, client *SemaphoreClient, cache *Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]*healthCheck{
			"semaphore_reachable": nil,
			"semaphore_api":       nil,
			"api_key_valid":       nil,
			"cache":               nil,
		}

		// --- Check 1: Semaphore reachable + API key valid ---
		// A successful GET /api/user means the host is reachable AND the token is valid
		user, err := client.GetUser()
		if err != nil {
			// Try to distinguish connection error from auth error
			checks["semaphore_reachable"] = statusFail(err.Error())
			checks["semaphore_api"] = statusFail("could not reach API")
			checks["api_key_valid"] = statusFail("unknown — API unreachable")
		} else {
			checks["semaphore_reachable"] = statusOK()
			checks["semaphore_api"] = statusOK()
			if user.ID == 0 {
				checks["api_key_valid"] = statusFail("API returned empty user")
			} else {
				checks["api_key_valid"] = statusOK()
			}
		}

		// --- Check 2: Cache available and populated ---
		data := cache.Get()
		if data == nil || data.LastUpdated.IsZero() {
			checks["cache"] = statusFail("cache is empty — no successful fetch yet")
		} else {
			checks["cache"] = &healthCheck{
				Status:  "ok",
				Message: "last updated " + cache.Age().Truncate(time.Second).String() + " ago",
			}
		}

		// Overall status — fail if any check failed
		overall := "ok"
		for _, c := range checks {
			if c.Status == "fail" {
				overall = "fail"
				break
			}
		}

		resp := healthStatus{
			Status:    overall,
			Version:   Version,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    checks,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if overall == "fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(resp)
	}
}
