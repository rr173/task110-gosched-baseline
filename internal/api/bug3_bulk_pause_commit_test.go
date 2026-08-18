package api

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/model"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
	"testing"
)

func TestBulkPausePersistsAcrossHandlerTransaction(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := scheduler.New(st, clock.RealClock{}, scheduler.DefaultExecutor{}, metrics.New(), 0)
	task := &model.Task{ID: store.NewID("task"), Name: "ops", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true, Tags: []string{"ops"}}
	if err := s.ScheduleNew(task); err != nil {
		t.Fatal(err)
	}
	h := NewMux(s, st, metrics.New(), "")
	req := httptest.NewRequest("POST", "/api/v1/tasks/bulk-pause", bytes.NewBufferString(`{"tag":"ops"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	tx, _ := st.BeginTx()
	got, err := st.GetTask(tx, task.ID)
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Fatalf("bulk pause was not durable: %+v", got)
	}
}
