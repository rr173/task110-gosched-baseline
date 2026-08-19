// Package model defines the domain types for the scheduler engine: tasks,
// runs (per-execution records), tags and aggregate stats. It also carries the
// validation and "due" logic used by the store and scheduler.
package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"task110-gosched/internal/cron"
)

// SpecType enumerates how a task is scheduled.
type SpecType string

const (
	SpecCron    SpecType = "cron"
	SpecOnce    SpecType = "once"
	SpecDelayed SpecType = "delayed"
)

// TaskKind enumerates the executor behaviour for a task.
type TaskKind string

const (
	KindNoop TaskKind = "noop" // always succeeds, output "ok"
	KindEcho TaskKind = "echo" // succeeds, output == payload
	KindHTTP TaskKind = "http" // simulated HTTP call (no real network)
	KindFail TaskKind = "fail" // deterministic failure (for retry tests)
)

// RunStatus enumerates the lifecycle of a single execution.
type RunStatus string

const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunOrphaned  RunStatus = "orphaned" // was running when the process died
)

// Trigger reason for a run.
type Trigger string

const (
	TriggerSchedule Trigger = "schedule"
	TriggerManual   Trigger = "manual"
	TriggerRetry    Trigger = "retry"
)

// ErrInvalid is returned for user-facing validation failures.
var ErrInvalid = errors.New("invalid task")

// Task is a scheduled unit of work.
type Task struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           TaskKind `json:"kind"`
	SpecType       SpecType `json:"spec_type"`
	CronExpr       string   `json:"cron_expr,omitempty"`
	DelaySeconds   int64    `json:"delay_seconds,omitempty"`
	Payload        string   `json:"payload,omitempty"`
	Enabled        bool     `json:"enabled"`
	Paused         bool     `json:"paused"`
	Tags           []string `json:"tags,omitempty"`
	MaxRetries     int      `json:"max_retries"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	CreatedAt      int64    `json:"created_at"`  // unix ms
	UpdatedAt      int64    `json:"updated_at"`  // unix ms
	NextRunAt      int64    `json:"next_run_at"` // unix ms; 0 means none
	LastRunAt      int64    `json:"last_run_at"` // unix ms; 0 means never
	Version        int64    `json:"version"`
}

// Run is one execution of a task.
type Run struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	TaskName    string    `json:"task_name,omitempty"`
	Attempt     int       `json:"attempt"`
	Status      RunStatus `json:"status"`
	ScheduledAt int64     `json:"scheduled_at"` // unix ms
	StartedAt   int64     `json:"started_at"`   // unix ms
	FinishedAt  int64     `json:"finished_at"`  // unix ms
	TriggeredBy Trigger   `json:"triggered_by"`
	Error       string    `json:"error,omitempty"`
	Output      string    `json:"output,omitempty"`
}

// Tag groups tasks.
type Tag struct {
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
}

// DailyStat aggregates run outcomes per calendar day.
type DailyStat struct {
	Day       string `json:"day"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Running   int    `json:"running"`
}

// Stats is the overall aggregate view.
type Stats struct {
	TasksTotal    int `json:"tasks_total"`
	TasksActive   int `json:"tasks_active"`
	TasksPaused   int `json:"tasks_paused"`
	TasksDisabled int `json:"tasks_disabled"`
	RunsTotal     int `json:"runs_total"`
	RunsSucceeded int `json:"runs_succeeded"`
	RunsFailed    int `json:"runs_failed"`
	RunsOrphaned  int `json:"runs_orphaned"`
	RunsRunning   int `json:"runs_running"`
}

// Status returns the derived lifecycle status of a task.
func (t *Task) Status() string {
	switch {
	case !t.Enabled:
		return "disabled"
	case t.Paused:
		return "paused"
	default:
		return "active"
	}
}

// IsDue reports whether the task should be dispatched at nowMs.
func (t *Task) IsDue(nowMs int64) bool {
	if !t.Enabled || t.Paused {
		return false
	}
	return t.NextRunAt > 0 && t.NextRunAt <= nowMs
}

// Validate performs structural validation independent of a clock.
func (t *Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	switch t.Kind {
	case KindNoop, KindEcho, KindHTTP, KindFail:
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, t.Kind)
	}
	switch t.SpecType {
	case SpecCron:
		if strings.TrimSpace(t.CronExpr) == "" {
			return fmt.Errorf("%w: cron_expr is required for cron spec", ErrInvalid)
		}
		if _, err := cron.Parse(t.CronExpr); err != nil {
			return fmt.Errorf("%w: invalid cron_expr: %v", ErrInvalid, err)
		}
	case SpecOnce:
		// scheduled time supplied via NextRunAt at creation
	case SpecDelayed:
		if t.DelaySeconds <= 0 {
			return fmt.Errorf("%w: delay_seconds must be > 0 for delayed spec", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown spec_type %q", ErrInvalid, t.SpecType)
	}
	if t.MaxRetries < 0 {
		return fmt.Errorf("%w: max_retries must be >= 0", ErrInvalid)
	}
	if t.TimeoutSeconds < 0 {
		return fmt.Errorf("%w: timeout_seconds must be >= 0", ErrInvalid)
	}
	return nil
}

// ComputeNextCron returns the next execution time for a cron task after from.
func (t *Task) ComputeNextCron(from time.Time) (time.Time, error) {
	if t.SpecType != SpecCron {
		return time.Time{}, fmt.Errorf("%w: not a cron task", ErrInvalid)
	}
	s, err := cron.Parse(t.CronExpr)
	if err != nil {
		return time.Time{}, err
	}
	// BUG7 fault: converting the caller's calendar to UTC changes the local
	// day on which a cron expression is evaluated.
	next := s.Next(from.UTC())
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("%w: no next cron time within limit", ErrInvalid)
	}
	return next, nil
}

// TagsCSV encodes tags as a deduplicated, comma-joined string for storage.
func TagsCSV(tags []string) string {
	seen := make(map[string]struct{}, len(tags))
	clean := make([]string, 0, len(tags))
	for _, tg := range tags {
		tg = strings.TrimSpace(tg)
		if tg == "" {
			continue
		}
		if _, dup := seen[tg]; dup {
			continue
		}
		seen[tg] = struct{}{}
		clean = append(clean, tg)
	}
	return strings.Join(clean, ",")
}
