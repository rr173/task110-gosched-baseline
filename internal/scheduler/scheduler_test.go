package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
)

func newTestScheduler(t *testing.T) (*Scheduler, *store.Store, *clock.FakeClock) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.db")
	st, err := store.Open(p)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	clk := clock.NewFakeClock(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	s := New(st, clk, DefaultExecutor{}, metrics.New(), time.Hour)
	return s, st, clk
}

func TestScheduleNewComputesNextRun(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	tk := &model.Task{Name: "cron", Kind: model.KindNoop, SpecType: model.SpecCron, CronExpr: "0 12 * * *"}
	if err := s.ScheduleNew(tk); err != nil {
		t.Fatalf("ScheduleNew: %v", err)
	}
	tx, _ := st.BeginTx()
	got, _ := st.GetTask(tx, tk.ID)
	tx.Rollback()
	// next run should be today at 12:00 (future relative to 00:00).
	want := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	if got.NextRunAt != want {
		t.Fatalf("next_run_at=%d want %d", got.NextRunAt, want)
	}
}

func TestTickExecutesDueTask(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	tk := &model.Task{Name: "once", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	if err := s.ScheduleNew(tk); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	tx, _ := st.BeginTx()
	runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
	got, _ := st.GetTask(tx, tk.ID)
	tx.Rollback()
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Status != model.RunSucceeded {
		t.Fatalf("run status=%s", runs[0].Status)
	}
	if got.NextRunAt != 0 {
		t.Fatalf("once task should have next_run_at=0, got %d", got.NextRunAt)
	}
}

func TestRecoverOrphanAndRetry(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	// A failing task with retry budget.
	tk := &model.Task{Name: "fail", Kind: model.KindFail, SpecType: model.SpecOnce, MaxRetries: 2}
	if err := s.ScheduleNew(tk); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash: insert a run left in "running".
	tx, _ := st.BeginTx()
	orphan := &model.Run{ID: store.NewID("run"), TaskID: tk.ID, TaskName: tk.Name, Attempt: 1, Status: model.RunRunning, ScheduledAt: 1, StartedAt: 1}
	if err := st.CreateRun(tx, orphan); err != nil {
		t.Fatal(err)
	}
	tx.Commit()

	n, err := s.Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered %d orphans, want 1", n)
	}
	tx, _ = st.BeginTx()
	runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
	tx.Rollback()
	// Original marked orphaned + one retry run created (attempt 2).
	var orphaned, retried bool
	for _, r := range runs {
		if r.Status == model.RunOrphaned && r.Attempt == 1 {
			orphaned = true
		}
		if r.Attempt == 2 {
			retried = true
		}
	}
	if !orphaned {
		t.Fatal("original run not marked orphaned")
	}
	if !retried {
		t.Fatal("retry run not created")
	}
}

func TestTriggerNow(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	tk := &model.Task{Name: "t", Kind: model.KindEcho, SpecType: model.SpecOnce, Payload: "hello"}
	if err := s.ScheduleNew(tk); err != nil {
		t.Fatal(err)
	}
	// Disable so schedule wouldn't fire, but manual trigger still works.
	tx, _ := st.BeginTx()
	_ = st.SetEnabled(tx, tk.ID, false, 1)
	tx.Commit()
	if err := s.TriggerNow(tk.ID); err != nil {
		t.Fatalf("TriggerNow: %v", err)
	}
	tx, _ = st.BeginTx()
	runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
	tx.Rollback()
	if len(runs) != 1 || runs[0].TriggeredBy != model.TriggerManual {
		t.Fatalf("manual run missing: %+v", runs)
	}
}
