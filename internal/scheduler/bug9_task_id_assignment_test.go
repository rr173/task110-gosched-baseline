package scheduler

import (
	"task110-gosched/internal/model"
	"testing"
)

func TestScheduleNewAssignsDurableIDsAcrossSchedulerAndModel(t *testing.T) {
	s, st, _ := newTestScheduler(t)
	a := &model.Task{Name: "first", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	b := &model.Task{Name: "second", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	if err := s.ScheduleNew(a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" {
		t.Fatal("ScheduleNew returned task without an ID")
	}
	if err := s.ScheduleNew(b); err != nil {
		t.Fatal(err)
	}
	if b.ID == "" || a.ID == b.ID {
		t.Fatalf("invalid IDs: %q %q", a.ID, b.ID)
	}
	tx, _ := st.BeginTx()
	defer tx.Rollback()
	if got, err := st.GetTask(tx, a.ID); err != nil || got == nil {
		t.Fatalf("first task not addressable: %v %+v", err, got)
	}
	if err := st.CreateTask(tx, &model.Task{Name: "missing-id", Kind: model.KindNoop, SpecType: model.SpecOnce}); err == nil {
		t.Fatal("store accepted a task without a durable ID")
	}
}
