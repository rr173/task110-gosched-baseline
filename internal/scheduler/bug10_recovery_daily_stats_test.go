package scheduler

import (
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
	"testing"
	"time"
)

func TestRecoveryRecordsOrphanOutcomeInDailyStatsAcrossSchedulerAndStore(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	task := &model.Task{Name: "orphan", Kind: model.KindNoop, SpecType: model.SpecOnce, MaxRetries: 0}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)
	runID := store.NewID("run")
	tx, _ := st.BeginTx()
	if err := st.CreateRun(tx, &model.Run{ID: runID, TaskID: task.ID, TaskName: task.Name, Attempt: 1, Status: model.RunRunning, ScheduledAt: day.UnixMilli(), StartedAt: day.UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recover(); err != nil {
		t.Fatal(err)
	}
	tx, _ = st.BeginTx()
	stats, err := st.DailyStats(tx, "2026-06-01", "2026-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Failed != 1 {
		t.Fatalf("orphan missing from daily stats: %+v", stats)
	}
	run, err := st.GetRun(tx, runID)
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if run == nil || run.Status != model.RunOrphaned || run.Error == "" {
		t.Fatalf("orphan run was not persisted with its outcome: %+v", run)
	}
}
