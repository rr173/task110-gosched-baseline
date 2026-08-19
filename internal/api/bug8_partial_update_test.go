package api

import (
	"encoding/json"
	"task110-gosched/internal/model"
	"testing"
)

func TestPartialTaskUpdatePreservesScheduleFieldsAcrossAPIAndModel(t *testing.T) {
	direct := &model.Task{SpecType: model.SpecCron, CronExpr: "0 12 * * *", DelaySeconds: 9, NextRunAt: 123}
	direct.ApplySchedulePatch(model.SchedulePatch{})
	if direct.CronExpr == "" || direct.DelaySeconds == 0 || direct.NextRunAt == 0 {
		t.Fatalf("model patch erased omitted schedule fields: %+v", direct)
	}
	srv, _ := newTestServer(t)
	resp, body := doJSON(t, srv, "POST", "/api/v1/tasks", map[string]interface{}{"name": "cron", "kind": "noop", "spec_type": "cron", "cron_expr": "0 12 * * *"})
	if resp.StatusCode != 201 {
		t.Fatalf("create=%d %s", resp.StatusCode, body)
	}
	var created map[string]interface{}
	_ = json.Unmarshal(body, &created)
	id := created["id"].(string)
	resp, body = doJSON(t, srv, "PUT", "/api/v1/tasks/"+id, map[string]interface{}{"name": "renamed"})
	if resp.StatusCode != 200 {
		t.Fatalf("partial update=%d %s", resp.StatusCode, body)
	}
	resp, body = doJSON(t, srv, "GET", "/api/v1/tasks/"+id, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get=%d %s", resp.StatusCode, body)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(body, &got)
	if got["cron_expr"] != "0 12 * * *" {
		t.Fatalf("schedule was cleared: %+v", got)
	}
}
