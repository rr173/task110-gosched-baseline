// Package metrics holds in-process counters for the scheduler engine. They are
// cheap to update on the hot path (dispatch/retry) and surfaced via the
// /api/v1/metrics endpoint. They are not durable: persistence of outcomes lives
// in stats_daily.
package metrics

import (
	"sync"
	"sync/atomic"

	"task110-gosched/internal/model"
)

// Metrics aggregates runtime counters.
type Metrics struct {
	tasksCreated  int64
	runsTotal     int64
	runsSucceeded int64
	runsFailed    int64
	runsOrphaned  int64
	runsRetried   int64
	dispatches    int64
	mu            sync.Mutex // only guards fields not using atomics below
	byKind        map[model.TaskKind]int64
	byStatus      map[model.RunStatus]int64
}

// New returns an empty Metrics.
func New() *Metrics {
	return &Metrics{
		byKind:   map[model.TaskKind]int64{},
		byStatus: map[model.RunStatus]int64{},
	}
}

// IncTasksCreated counts a created task.
func (m *Metrics) IncTasksCreated() { atomic.AddInt64(&m.tasksCreated, 1) }

// IncDispatches counts a scheduler dispatch cycle.
func (m *Metrics) IncDispatches() { atomic.AddInt64(&m.dispatches, 1) }

// IncRetried counts a retry enqueue.
func (m *Metrics) IncRetried() { atomic.AddInt64(&m.runsRetried, 1) }

// RecordRun counts a finished run by status and kind.
func (m *Metrics) RecordRun(status model.RunStatus, kind model.TaskKind) {
	atomic.AddInt64(&m.runsTotal, 1)
	switch status {
	case model.RunSucceeded:
		atomic.AddInt64(&m.runsSucceeded, 1)
	case model.RunFailed, model.RunOrphaned:
		atomic.AddInt64(&m.runsFailed, 1)
		if status == model.RunOrphaned {
			atomic.AddInt64(&m.runsOrphaned, 1)
		}
	}
	m.byKind[kind]++
	m.byStatus[status]++
}

// Snapshot is a point-in-time copy for serialization.
type Snapshot struct {
	TasksCreated  int64            `json:"tasks_created"`
	RunsTotal     int64            `json:"runs_total"`
	RunsSucceeded int64            `json:"runs_succeeded"`
	RunsFailed    int64            `json:"runs_failed"`
	RunsOrphaned  int64            `json:"runs_orphaned"`
	RunsRetried   int64            `json:"runs_retried"`
	Dispatches    int64            `json:"dispatches"`
	ByKind        map[string]int64 `json:"by_kind"`
	ByStatus      map[string]int64 `json:"by_status"`
}

// Snapshot returns a consistent copy of the counters.
func (m *Metrics) Snapshot() Snapshot {
	byKind := make(map[string]int64, len(m.byKind))
	for k, v := range m.byKind {
		byKind[string(k)] = v
	}
	byStatus := make(map[string]int64, len(m.byStatus))
	for k, v := range m.byStatus {
		byStatus[string(k)] = v
	}
	return Snapshot{
		TasksCreated:  atomic.LoadInt64(&m.tasksCreated),
		RunsTotal:     atomic.LoadInt64(&m.runsTotal),
		RunsSucceeded: atomic.LoadInt64(&m.runsSucceeded),
		RunsFailed:    atomic.LoadInt64(&m.runsFailed),
		RunsOrphaned:  atomic.LoadInt64(&m.runsOrphaned),
		RunsRetried:   atomic.LoadInt64(&m.runsRetried),
		Dispatches:    atomic.LoadInt64(&m.dispatches),
		ByKind:        byKind,
		ByStatus:      byStatus,
	}
}
