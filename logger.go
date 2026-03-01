package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// ecsHandler is a slog.Handler that writes log records in Elastic Common Schema
// (ECS) 1.x JSON format, compatible with Wazuh and other ECS-aware SIEM systems.
//
// ECS field reference: https://www.elastic.co/guide/en/ecs/current/ecs-field-reference.html
type ecsHandler struct {
	w     io.Writer
	level slog.Level
	attrs []slog.Attr
}

// newECSHandler creates a new ECS JSON handler writing to w.
func newECSHandler(w io.Writer, level slog.Level) *ecsHandler {
	return &ecsHandler{w: w, level: level}
}

func (h *ecsHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *ecsHandler) Handle(_ context.Context, r slog.Record) error {
	// Map slog levels to ECS log.level values
	ecsLevel := "info"
	switch {
	case r.Level >= slog.LevelError:
		ecsLevel = "error"
	case r.Level >= slog.LevelWarn:
		ecsLevel = "warn"
	case r.Level >= slog.LevelInfo:
		ecsLevel = "info"
	default:
		ecsLevel = "debug"
	}

	entry := map[string]any{
		// ECS base fields
		"@timestamp": r.Time.UTC().Format(time.RFC3339Nano),
		"log": map[string]any{
			"level": ecsLevel,
		},
		"message": r.Message,
		"ecs": map[string]any{
			"version": "1.12.0",
		},
		// Service identity — useful for filtering in Wazuh/Kibana
		"service": map[string]any{
			"name": "semaphore-prometheus-exporter",
			"type": "metrics",
		},
	}

	// Attach caller info to log.origin if available
	if r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := frames.Next()
		if f.File != "" {
			entry["log"].(map[string]any)["origin"] = map[string]any{
				"file": map[string]any{
					"name": f.File,
					"line": f.Line,
				},
				"function": f.Function,
			}
		}
	}

	// Attach handler-level attrs
	for _, a := range h.attrs {
		applyAttr(entry, a)
	}

	// Attach record-level attrs
	r.Attrs(func(a slog.Attr) bool {
		applyAttr(entry, a)
		return true
	})

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = h.w.Write(data)
	return err
}

func (h *ecsHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &ecsHandler{w: h.w, level: h.level, attrs: newAttrs}
}

func (h *ecsHandler) WithGroup(name string) slog.Handler {
	// Groups not needed for ECS flat structure
	return h
}

// applyAttr maps a slog.Attr to well-known ECS fields where possible,
// otherwise writes it as a top-level key.
func applyAttr(entry map[string]any, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}

	switch a.Key {
	case "error":
		entry["error"] = map[string]any{"message": a.Value.String()}
	case "url":
		entry["url"] = map[string]any{"full": a.Value.String()}
	case "http.status":
		ensureMap(entry, "http")
		ensureMap(entry["http"].(map[string]any), "response")
		entry["http"].(map[string]any)["response"].(map[string]any)["status_code"] = a.Value.Any()
	case "duration":
		entry["event"] = map[string]any{"duration": a.Value.Duration().Nanoseconds()}
	case "project_id":
		ensureMap(entry, "labels")
		entry["labels"].(map[string]any)["project_id"] = a.Value.String()
	case "task_id":
		ensureMap(entry, "labels")
		entry["labels"].(map[string]any)["task_id"] = a.Value.String()
	default:
		entry[a.Key] = a.Value.Any()
	}
}

func ensureMap(m map[string]any, key string) {
	if _, ok := m[key]; !ok {
		m[key] = map[string]any{}
	}
}

// setupLogger initialises the global slog logger with the ECS JSON handler.
// Log level is controlled via the LOG_LEVEL environment variable
// (debug, info, warn, error). Default is info.
func setupLogger() {
	level := slog.LevelInfo
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := newECSHandler(os.Stdout, level)
	slog.SetDefault(slog.New(handler))
}
