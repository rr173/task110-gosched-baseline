package metrics

import (
	"testing"

	"task110-gosched/internal/model"
)

func TestRecordRunAndSnapshot(t *testing.T) {
	m := New()
	m.IncTasksCreated()
	m.RecordRun(model.RunSucceeded, model.KindNoop)
	m.RecordRun(model.RunFailed, model.KindFail)
	m.IncRetried()

	s := m.Snapshot()
	if s.TasksCreated != 1 {
		t.Fatalf("tasks_created=%d", s.TasksCreated)
	}
	if s.RunsTotal != 2 {
		t.Fatalf("runs_total=%d", s.RunsTotal)
	}
	if s.RunsSucceeded != 1 || s.RunsFailed != 1 {
		t.Fatalf("breakdown wrong: %+v", s)
	}
	if s.RunsRetried != 1 {
		t.Fatalf("retried=%d", s.RunsRetried)
	}
	if s.ByKind[string(model.KindFail)] != 1 {
		t.Fatalf("by_kind fail=%d", s.ByKind[string(model.KindFail)])
	}
}
