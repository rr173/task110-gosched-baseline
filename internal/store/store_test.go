package store

import (
	"path/filepath"
	"testing"
	"time"

	"task110-gosched/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.db")
	s, err := Open(p)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTask(name string) *model.Task {
	now := time.Now().UnixMilli()
	return &model.Task{
		ID:        newID("task"),
		Name:      name,
		Kind:      model.KindNoop,
		SpecType:  model.SpecOnce,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestTaskCRUD(t *testing.T) {
	s := openTest(t)
	tx, _ := s.BeginTx()
	tk := newTask("alpha")
	if err := s.CreateTask(tx, tk); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := s.GetTask(tx, tk.ID)
	if err != nil || got == nil {
		t.Fatalf("GetTask: %v %v", got, err)
	}
	if got.Name != "alpha" {
		t.Fatalf("name mismatch: %q", got.Name)
	}
	got.Name = "beta"
	got.UpdatedAt = time.Now().UnixMilli()
	if err := s.UpdateTask(tx, got); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got2, _ := s.GetTask(tx, tk.ID)
	if got2.Name != "beta" {
		t.Fatalf("update not persisted: %q", got2.Name)
	}
	if err := s.DeleteTask(tx, tk.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	gone, _ := s.GetTask(tx, tk.ID)
	if gone != nil {
		t.Fatal("task should be deleted")
	}
	_ = tx.Commit()
}

func TestListAndFilters(t *testing.T) {
	s := openTest(t)
	tx, _ := s.BeginTx()
	a := newTask("a")
	a.Tags = []string{"x", "y"}
	a.Paused = true
	b := newTask("b")
	b.Tags = []string{"x"}
	if err := s.CreateTask(tx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(tx, b); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit()

	tx, _ = s.BeginTx()
	defer tx.Rollback()
	byTag, err := s.ListTasks(tx, TaskFilter{Tag: "x"})
	if err != nil || len(byTag) != 2 {
		t.Fatalf("byTag: %v len=%d", err, len(byTag))
	}
	paused, err := s.ListTasks(tx, TaskFilter{Status: "paused"})
	if err != nil || len(paused) != 1 {
		t.Fatalf("paused: %v len=%d", err, len(paused))
	}
}

func TestRunLifecycleAndRetry(t *testing.T) {
	s := openTest(t)
	tx, _ := s.BeginTx()
	tk := newTask("r")
	if err := s.CreateTask(tx, tk); err != nil {
		t.Fatal(err)
	}
	r := &model.Run{ID: newID("run"), TaskID: tk.ID, TaskName: tk.Name, Attempt: 1, Status: model.RunRunning, ScheduledAt: 1}
	if err := s.CreateRun(tx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRunStatus(tx, r.ID, model.RunFailed, 1, 2, "", "boom"); err != nil {
		t.Fatal(err)
	}
	re, err := s.RetryRun(tx, r, 3)
	if err != nil {
		t.Fatal(err)
	}
	if re.Attempt != 2 || re.TriggeredBy != model.TriggerRetry || re.Status != model.RunPending {
		t.Fatalf("retry run wrong: %+v", re)
	}
	_ = tx.Commit()
}

func TestStatsAndTags(t *testing.T) {
	s := openTest(t)
	tx, _ := s.BeginTx()
	if err := s.CreateTag(tx, "ops", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRunOutcome(tx, "2026-05-01", model.RunSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRunOutcome(tx, "2026-05-01", model.RunFailed); err != nil {
		t.Fatal(err)
	}
	daily, err := s.DailyStats(tx, "2026-05-01", "2026-05-01")
	if err != nil || len(daily) != 1 {
		t.Fatalf("daily: %v %d", err, len(daily))
	}
	if daily[0].Succeeded != 1 || daily[0].Failed != 1 {
		t.Fatalf("daily wrong: %+v", daily[0])
	}
	st, err := s.OverallStats(tx)
	if err != nil {
		t.Fatal(err)
	}
	_ = st
	_ = tx.Commit()
}
