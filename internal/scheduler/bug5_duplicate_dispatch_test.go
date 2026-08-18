package scheduler

import (
	"context"
	"sync"
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
	"testing"
	"time"
)

type multiDispatchGate struct {
	started chan struct{}
	release chan struct{}
}

func (e *multiDispatchGate) Execute(context.Context, *model.Task, *model.Run) (string, error) {
	e.started <- struct{}{}
	<-e.release
	return "ok", nil
}

func TestConcurrentDispatchClaimsTaskOnceAcrossSchedulerAndStore(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_ = base
	gate := &multiDispatchGate{started: make(chan struct{}, 2), release: make(chan struct{})}
	s, st, _ := newTestScheduler(t)
	s.exec = gate
	task := &model.Task{Name: "single-dispatch", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.dispatch(task, task.NextRunAt) }()
	}
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch did not start")
	}
	close(gate.release)
	wg.Wait()
	tx, _ := st.BeginTx()
	runs, err := st.ListRuns(tx, store.RunFilter{TaskID: task.ID})
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("created %d runs: %+v", len(runs), runs)
	}
}
