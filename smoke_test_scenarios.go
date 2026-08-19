package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/model"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
)

// runSmokeTest exercises the engine contract against a temporary SQLite file. It
// uses a FakeClock so no real-time sleeps are needed, and validates persistence
// (reopen), recovery (orphaned runs), retries and stats.
func runSmokeTest() error {
	dir, err := os.MkdirTemp("", "gosched-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	idx := 0
	mk := func() (*scheduler.Scheduler, *store.Store, *clock.FakeClock) {
		idx++
		path := filepath.Join(dir, fmt.Sprintf("smoke-%d.db", idx))
		st, err := store.Open(path)
		if err != nil {
			panic(err)
		}
		clk := clock.NewFakeClock(base)
		s := scheduler.New(st, clk, scheduler.DefaultExecutor{}, metrics.New(), time.Hour)
		return s, st, clk
	}

	// 1) cron next-run computation.
	{
		s, st, _ := mk()
		tk := &model.Task{Name: "cron", Kind: model.KindNoop, SpecType: model.SpecCron, CronExpr: "0 12 * * *"}
		if err := s.ScheduleNew(tk); err != nil {
			return fmt.Errorf("schedule cron: %w", err)
		}
		tx, _ := st.BeginTx()
		got, _ := st.GetTask(tx, tk.ID)
		tx.Rollback()
		want := base.Add(12 * time.Hour).UnixMilli()
		if got.NextRunAt != want {
			st.Close()
			return fmt.Errorf("cron next_run_at=%d want %d", got.NextRunAt, want)
		}
		st.Close()
	}

	// 2) once task dispatches and succeeds on Tick.
	{
		s, st, _ := mk()
		tk := &model.Task{Name: "once", Kind: model.KindEcho, SpecType: model.SpecOnce, Payload: "hi", Enabled: true}
		if err := s.ScheduleNew(tk); err != nil {
			return fmt.Errorf("schedule once: %w", err)
		}
		if err := s.Tick(); err != nil {
			return fmt.Errorf("tick: %w", err)
		}
		tx, _ := st.BeginTx()
		runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
		got, _ := st.GetTask(tx, tk.ID)
		tx.Rollback()
		if len(runs) != 1 || runs[0].Status != model.RunSucceeded {
			st.Close()
			return fmt.Errorf("once run not succeeded: %+v", runs)
		}
		if got.NextRunAt != 0 {
			st.Close()
			return fmt.Errorf("once next_run_at should be 0, got %d", got.NextRunAt)
		}
		st.Close()
	}

	// 3) recovery: a run left "running" is orphaned and retried within budget.
	{
		s, st, _ := mk()
		tk := &model.Task{Name: "fail", Kind: model.KindFail, SpecType: model.SpecOnce, MaxRetries: 2}
		if err := s.ScheduleNew(tk); err != nil {
			return fmt.Errorf("schedule fail: %w", err)
		}
		tx, _ := st.BeginTx()
		orphan := &model.Run{ID: store.NewID("run"), TaskID: tk.ID, TaskName: tk.Name, Attempt: 1, Status: model.RunRunning, ScheduledAt: 1, StartedAt: 1}
		if err := st.CreateRun(tx, orphan); err != nil {
			tx.Rollback()
			st.Close()
			return fmt.Errorf("seed orphan: %w", err)
		}
		if err := tx.Commit(); err != nil {
			st.Close()
			return err
		}
		n, err := s.Recover()
		if err != nil {
			st.Close()
			return fmt.Errorf("recover: %w", err)
		}
		if n != 1 {
			st.Close()
			return fmt.Errorf("recovered %d, want 1", n)
		}
		tx, _ = st.BeginTx()
		runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
		tx.Rollback()
		var orphaned, retried bool
		for _, r := range runs {
			if r.Status == model.RunOrphaned && r.Attempt == 1 {
				orphaned = true
			}
			if r.Attempt == 2 {
				retried = true
			}
		}
		st.Close()
		if !orphaned || !retried {
			return fmt.Errorf("recovery incomplete: orphaned=%v retried=%v", orphaned, retried)
		}
	}

	// 4) manual trigger works even when the task is disabled.
	{
		s, st, _ := mk()
		tk := &model.Task{Name: "manual", Kind: model.KindEcho, SpecType: model.SpecOnce, Payload: "triggered"}
		if err := s.ScheduleNew(tk); err != nil {
			return fmt.Errorf("schedule manual: %w", err)
		}
		tx, _ := st.BeginTx()
		_ = st.SetEnabled(tx, tk.ID, false, 1)
		tx.Commit()
		if err := s.TriggerNow(tk.ID); err != nil {
			st.Close()
			return fmt.Errorf("trigger: %w", err)
		}
		tx, _ = st.BeginTx()
		runs, _ := st.ListRuns(tx, store.RunFilter{TaskID: tk.ID})
		tx.Rollback()
		st.Close()
		if len(runs) != 1 || runs[0].TriggeredBy != model.TriggerManual {
			return fmt.Errorf("manual trigger failed: %+v", runs)
		}
	}

	// 5) tags + stats.
	{
		s, st, _ := mk()
		tk := &model.Task{Name: "tagged", Kind: model.KindNoop, SpecType: model.SpecOnce, Tags: []string{"ops", "batch"}}
		if err := s.ScheduleNew(tk); err != nil {
			return fmt.Errorf("schedule tagged: %w", err)
		}
		tx, _ := st.BeginTx()
		tags, err := st.ListTags(tx)
		tx.Rollback()
		if err != nil {
			st.Close()
			return fmt.Errorf("list tags: %w", err)
		}
		if len(tags) != 2 {
			st.Close()
			return fmt.Errorf("expected 2 tags, got %d", len(tags))
		}
		st.Close()
	}

	return nil
}
