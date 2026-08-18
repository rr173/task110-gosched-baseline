package scheduler

import (
	"testing"

	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
)

func TestRunFailurePersistsErrorAcrossSchedulerAndStore(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	task := &model.Task{Name: "persist-failure", Kind: model.KindFail, SpecType: model.SpecOnce, Enabled: true}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	tx, err := st.BeginTx()
	if err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(tx, store.RunFilter{TaskID: task.ID})
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Error == "" {
		t.Fatalf("failed run lost error: %+v", runs)
	}
}
