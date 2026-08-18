package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
)

type dispatchGateExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *dispatchGateExecutor) Execute(_ context.Context, _ *model.Task, _ *model.Run) (string, error) {
	e.started <- struct{}{}
	<-e.release
	return "ok", nil
}

func TestConcurrentDispatchClaimsTaskOnce(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	gate := &dispatchGateExecutor{started: make(chan struct{}, 2), release: make(chan struct{})}
	s, st, clk := newTestScheduler(t)
	s.exec = gate
	_ = clk
	task := &model.Task{Name: "single-dispatch", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	if task.NextRunAt != base.UnixMilli() {
		t.Fatalf("unexpected next_run_at=%d", task.NextRunAt)
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.dispatch(task, task.NextRunAt); err != nil {
				t.Errorf("dispatch: %v", err)
			}
		}()
	}
	select {
	case <-gate.started:
	case <-time.After(2 * time.Second):
		t.Fatal("no dispatch reached executor")
	}
	close(gate.release)
	wg.Wait()

	tx, err := st.BeginTx()
	if err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(tx, store.RunFilter{TaskID: task.ID})
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("concurrent dispatch created %d runs: %+v", len(runs), runs)
	}
}
