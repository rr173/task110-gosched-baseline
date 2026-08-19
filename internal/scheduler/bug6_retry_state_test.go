package scheduler

import (
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
	"testing"
)

func TestRetryRejectsSuccessfulRunAcrossSchedulerAndRetryPolicy(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	task := &model.Task{Name: "retry-state", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true, MaxRetries: 2}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	tx, _ := st.BeginTx()
	runs, err := st.ListRuns(tx, store.RunFilter{TaskID: task.ID})
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatal(runs)
	}
	if err := s.RetryRunByID(runs[0].ID); err == nil {
		t.Fatal("successful run was retried")
	}

	// The persistence boundary must enforce the same lifecycle invariant even
	// when a caller bypasses the scheduler policy.
	tx, err = st.BeginTx()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RetryRun(tx, &runs[0], 123); err == nil {
		_ = tx.Rollback()
		t.Fatal("store accepted a successful run for retry")
	}
	_ = tx.Rollback()
}
