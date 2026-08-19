// Package api exposes the scheduler engine over HTTP. It wires the scheduler,
// store and metrics into a ServeMux with 20+ endpoints covering task lifecycle,
// runs, tags, stats, schedule preview and system endpoints.
package api

import (
	"net/http"

	"task110-gosched/internal/metrics"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
)

// Version is reported in logs and health output.
const Version = "task110-gosched/1.0"

// Server holds the dependencies for the HTTP handlers.
type Server struct {
	store      *store.Store
	sched      *scheduler.Scheduler
	metrics    *metrics.Metrics
	adminToken string
}

// NewMux builds the HTTP handler. Go 1.22+ method+path patterns are used, so
// this requires go >= 1.22 (the project pins 1.26.3).
func NewMux(sched *scheduler.Scheduler, st *store.Store, m *metrics.Metrics, adminToken string) http.Handler {
	s := &Server{store: st, sched: sched, metrics: m, adminToken: adminToken}
	mux := http.NewServeMux()

	// Task lifecycle
	mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/v1/tasks", s.handleListTasks)
	mux.HandleFunc("GET /api/v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("PUT /api/v1/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/enable", s.handleEnableTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/disable", s.handleDisableTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", s.handleTriggerTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", s.handlePauseTask)
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", s.handleResumeTask)
	mux.HandleFunc("GET /api/v1/tasks/{id}/runs", s.handleListTaskRuns)

	// Runs
	mux.HandleFunc("GET /api/v1/runs", s.handleListRuns)
	mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /api/v1/runs/{id}/retry", s.handleRetryRun)
	mux.HandleFunc("DELETE /api/v1/runs/{id}", s.handleDeleteRun)

	// Tags
	mux.HandleFunc("GET /api/v1/tags", s.handleListTags)
	mux.HandleFunc("POST /api/v1/tags", s.handleCreateTag)
	mux.HandleFunc("GET /api/v1/tags/{tag}/tasks", s.handleTagTasks)

	// Stats & schedule preview
	mux.HandleFunc("GET /api/v1/stats", s.handleStats)
	mux.HandleFunc("GET /api/v1/stats/daily", s.handleDailyStats)
	mux.HandleFunc("GET /api/v1/schedules/next", s.handleNextRuns)
	mux.HandleFunc("POST /api/v1/tasks/bulk-pause", s.handleBulkPause)

	// System
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/v1/report", s.handleReport)

	return panicRecovery(mux)
}
