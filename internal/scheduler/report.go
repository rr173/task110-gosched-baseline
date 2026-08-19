package scheduler

import (
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
)

type TaskReport struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
	Paused  int `json:"paused"`
	Due     int `json:"due"`
}
type SchedulerReport struct {
	Tasks   TaskReport       `json:"tasks"`
	Durable *model.Stats     `json:"durable"`
	Runtime metrics.Snapshot `json:"runtime"`
}

func (s *Scheduler) Report() (SchedulerReport, error) {
	tx, err := s.store.BeginTx()
	if err != nil {
		return SchedulerReport{}, err
	}
	defer tx.Rollback()
	tasks, err := s.store.ListTasks(tx, store.TaskFilter{})
	if err != nil {
		return SchedulerReport{}, err
	}
	durable, err := s.store.OverallStats(tx)
	if err != nil {
		return SchedulerReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return SchedulerReport{}, err
	}
	report := TaskReport{Total: len(tasks)}
	now := s.clock.Now().UnixMilli()
	for _, task := range tasks {
		if task.Enabled {
			report.Enabled++
		}
		if task.Paused {
			report.Paused++
		}
		if task.NextRunAt > 0 && task.NextRunAt <= now && task.Enabled && !task.Paused {
			report.Due++
		}
	}
	return SchedulerReport{Tasks: report, Durable: durable, Runtime: s.metrics.Snapshot()}, nil
}
