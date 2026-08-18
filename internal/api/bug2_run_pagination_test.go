package api

import (
	"encoding/json"
	"task110-gosched/internal/model"
	"task110-gosched/internal/store"
	"testing"
)

func TestRunPaginationPreservesLimitAndOffsetContract(t *testing.T) {
	srv, s := newTestServer(t)
	tx, _ := s.BeginTx()
	task := &model.Task{ID: store.NewID("task"), Name: "page", Kind: model.KindNoop, SpecType: model.SpecOnce, Enabled: true}
	if err := s.CreateTask(tx, task); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := s.CreateRun(tx, &model.Run{ID: store.NewID("run"), TaskID: task.ID, TaskName: task.Name, Attempt: i + 1, Status: model.RunSucceeded, ScheduledAt: int64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	resp, body := doJSON(t, srv, "GET", "/api/v1/runs?task_id="+task.ID+"&limit=2&offset=1", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list status=%d body=%s", resp.StatusCode, body)
	}
	var runs []model.Run
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].ScheduledAt != 3 || runs[1].ScheduledAt != 2 {
		t.Fatalf("wrong page: %+v", runs)
	}
}
