package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"task110-gosched/internal/model"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
)

// taskInput is the request body for create/update. Pointers distinguish
// "not provided" from "set to false" for booleans.
type taskInput struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	SpecType       string   `json:"spec_type"`
	CronExpr       string   `json:"cron_expr"`
	DelaySeconds   int64    `json:"delay_seconds"`
	Payload        string   `json:"payload"`
	Enabled        *bool    `json:"enabled"`
	Paused         *bool    `json:"paused"`
	Tags           []string `json:"tags"`
	MaxRetries     int      `json:"max_retries"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	NextRunAt      int64    `json:"next_run_at"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// requireAdmin returns true if the request carries a valid admin token. When no
// admin token is configured, auth is open (development mode).
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminToken == "" {
		return true
	}
	tok := r.Header.Get("X-Admin-Token")
	if tok == "" {
		tok = r.URL.Query().Get("admin_token")
	}
	if tok != s.adminToken {
		writeError(w, 401, "admin token required")
		return false
	}
	return true
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// --- Task lifecycle ---

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var in taskInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, 400, "invalid JSON body: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, 400, "name is required")
		return
	}
	t := &model.Task{
		ID:             store.NewID("task"),
		Name:           in.Name,
		Kind:           model.TaskKind(in.Kind),
		SpecType:       model.SpecType(in.SpecType),
		CronExpr:       in.CronExpr,
		DelaySeconds:   in.DelaySeconds,
		Payload:        in.Payload,
		Tags:           in.Tags,
		MaxRetries:     in.MaxRetries,
		TimeoutSeconds: in.TimeoutSeconds,
		Enabled:        in.Enabled == nil || *in.Enabled,
		Paused:         in.Paused != nil && *in.Paused,
	}
	if t.SpecType == model.SpecOnce {
		t.NextRunAt = in.NextRunAt
	}
	if err := s.sched.ScheduleNew(t); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.TaskFilter{
		Tag:    q.Get("tag"),
		Status: q.Get("status"),
		Limit:  atoiDefault(q.Get("limit"), 50),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	if v := q.Get("enabled"); v != "" {
		b := v == "true" || v == "1"
		f.Enabled = &b
	}
	if v := q.Get("paused"); v != "" {
		b := v == "true" || v == "1"
		f.Paused = &b
	}
	tx, err := s.store.BeginTx()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	ts, err := s.store.ListTasks(tx, f)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ts)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	t, err := s.store.GetTask(tx, id)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if t == nil {
		writeError(w, 404, "task not found")
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in taskInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, 400, "invalid JSON body: "+err.Error())
		return
	}
	tx, err := s.store.BeginTx()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	cur, err := s.store.GetTask(tx, id)
	if err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	if cur == nil {
		tx.Rollback()
		writeError(w, 404, "task not found")
		return
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	if in.Kind != "" {
		cur.Kind = model.TaskKind(in.Kind)
	}
	if in.SpecType != "" {
		cur.SpecType = model.SpecType(in.SpecType)
	}
	cur.CronExpr = in.CronExpr
	cur.DelaySeconds = in.DelaySeconds
	cur.Payload = in.Payload
	if in.Enabled != nil {
		cur.Enabled = *in.Enabled
	}
	if in.Paused != nil {
		cur.Paused = *in.Paused
	}
	if in.Tags != nil {
		cur.Tags = in.Tags
	}
	if in.MaxRetries != 0 {
		cur.MaxRetries = in.MaxRetries
	}
	if in.TimeoutSeconds != 0 {
		cur.TimeoutSeconds = in.TimeoutSeconds
	}
	if cur.SpecType == model.SpecOnce && in.NextRunAt != 0 {
		cur.NextRunAt = in.NextRunAt
	}
	cur.UpdatedAt = time.Now().UnixMilli()
	cur.Version++
	if err := cur.Validate(); err != nil {
		tx.Rollback()
		writeError(w, 400, err.Error())
		return
	}
	if err := s.store.UpdateTask(tx, cur); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	writeJSON(w, 200, cur)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	cur, _ := s.store.GetTask(tx, id)
	if cur == nil {
		tx.Rollback()
		writeError(w, 404, "task not found")
		return
	}
	if err := s.store.DeleteTask(tx, id); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	w.WriteHeader(204)
}

func (s *Server) setTaskEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	cur, _ := s.store.GetTask(tx, id)
	if cur == nil {
		tx.Rollback()
		writeError(w, 404, "task not found")
		return
	}
	if err := s.store.SetEnabled(tx, id, enabled, time.Now().UnixMilli()); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	writeJSON(w, 200, map[string]interface{}{"id": id, "enabled": enabled})
}

func (s *Server) handleEnableTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskEnabled(w, r, true)
}

func (s *Server) handleDisableTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskEnabled(w, r, false)
}

func (s *Server) setTaskPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	cur, _ := s.store.GetTask(tx, id)
	if cur == nil {
		tx.Rollback()
		writeError(w, 404, "task not found")
		return
	}
	if err := s.store.SetPaused(tx, id, paused, time.Now().UnixMilli()); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	writeJSON(w, 200, map[string]interface{}{"id": id, "paused": paused})
}

func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskPaused(w, r, true)
}

func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	s.setTaskPaused(w, r, false)
}

func (s *Server) handleTriggerTask(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.sched.TriggerNow(id); err != nil {
		if err == scheduler.ErrTaskNotFound {
			writeError(w, 404, "task not found")
			return
		}
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]interface{}{"id": id, "triggered": true})
}

func (s *Server) handleListTaskRuns(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f := store.RunFilter{TaskID: id, Limit: atoiDefault(r.URL.Query().Get("limit"), 50)}
	tx, _ := s.store.BeginTx()
	runs, err := s.store.ListRuns(tx, f)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, runs)
}

// --- Runs ---

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.RunFilter{
		TaskID: q.Get("task_id"),
		Status: q.Get("status"),
		Limit:  atoiDefault(q.Get("limit"), 50),
		Offset: atoiDefault(q.Get("offset"), 0),
	}
	tx, _ := s.store.BeginTx()
	runs, err := s.store.ListRuns(tx, f)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, runs)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	run, err := s.store.GetRun(tx, id)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if run == nil {
		writeError(w, 404, "run not found")
		return
	}
	writeJSON(w, 200, run)
}

func (s *Server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	if err := s.sched.RetryRunByID(id); err != nil {
		switch err {
		case scheduler.ErrRunNotFound, scheduler.ErrTaskNotFound:
			writeError(w, 404, err.Error())
		case scheduler.ErrNoRetryBudget:
			writeError(w, 409, err.Error())
		default:
			writeError(w, 500, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]interface{}{"id": id, "retried": true})
}

func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	tx, _ := s.store.BeginTx()
	if err := s.store.DeleteRun(tx, id); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	w.WriteHeader(204)
}

// --- Tags ---

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tx, _ := s.store.BeginTx()
	tags, err := s.store.ListTags(tx)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, tags)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, 400, "name is required")
		return
	}
	tx, _ := s.store.BeginTx()
	if err := s.store.CreateTag(tx, strings.TrimSpace(body.Name), time.Now().UnixMilli()); err != nil {
		tx.Rollback()
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	writeJSON(w, 201, map[string]string{"name": strings.TrimSpace(body.Name)})
}

func (s *Server) handleTagTasks(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	tx, _ := s.store.BeginTx()
	ts, err := s.store.TasksByTag(tx, tag)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ts)
}

// --- Stats & schedule preview ---

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	tx, _ := s.store.BeginTx()
	st, err := s.store.OverallStats(tx)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleDailyStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	now := time.Now().UTC()
	to := q.Get("to")
	if to == "" {
		to = now.Format("2006-01-02")
	}
	from := q.Get("from")
	if from == "" {
		from = now.AddDate(0, 0, -6).Format("2006-01-02")
	}
	tx, _ := s.store.BeginTx()
	ds, err := s.store.DailyStats(tx, from, to)
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ds)
}

func (s *Server) handleNextRuns(w http.ResponseWriter, r *http.Request) {
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	type preview struct {
		ID         string `json:"id"`
		NextRunAt  int64  `json:"next_run_at"`
		NextRunISO string `json:"next_run_iso"`
	}
	out := make([]preview, 0, len(ids))
	var seen map[string]bool
	tx, _ := s.store.BeginTx()
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if seen[id] {
			continue
		}
		t, err := s.store.GetTask(tx, id)
		if err != nil || t == nil {
			continue
		}
		if t.SpecType != model.SpecCron {
			continue
		}
		if n, err := t.ComputeNextCron(time.Now()); err == nil {
			out = append(out, preview{ID: id, NextRunAt: n.UnixMilli(), NextRunISO: n.Format(time.RFC3339)})
		}
		seen[id] = true
	}
	tx.Rollback()
	writeJSON(w, 200, out)
}

func (s *Server) handleBulkPause(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		Tag string `json:"tag"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Tag) == "" {
		writeError(w, 400, "tag is required")
		return
	}
	tx, _ := s.store.BeginTx()
	n, err := s.store.BulkPauseByTag(tx, strings.TrimSpace(body.Tag), time.Now().UnixMilli())
	tx.Rollback()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	tx.Commit()
	writeJSON(w, 200, map[string]interface{}{"tag": strings.TrimSpace(body.Tag), "paused": n})
}

// --- System ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok", "version": Version})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.metrics.Snapshot())
}

// panicRecovery wraps a handler to recover from panics and return 500 instead of
// crashing the server.
func panicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic in %s %s: %v", r.Method, r.URL.Path, rec)
				writeError(w, 500, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
