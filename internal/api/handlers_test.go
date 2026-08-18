package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"task110-gosched/internal/clock"
	"task110-gosched/internal/metrics"
	"task110-gosched/internal/scheduler"
	"task110-gosched/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "api.db")
	st, err := store.Open(p)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	sched := scheduler.New(st, clock.RealClock{}, scheduler.DefaultExecutor{}, metrics.New(), 0)
	srv := httptest.NewServer(NewMux(sched, st, metrics.New(), ""))
	t.Cleanup(srv.Close)
	return srv, st
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

func TestCreateAndGetTask(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, body := doJSON(t, srv, "POST", "/api/v1/tasks", map[string]interface{}{
		"name":      "daily-report",
		"kind":      "noop",
		"spec_type": "cron",
		"cron_expr": "0 0 * * *",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var created map[string]interface{}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id returned")
	}

	resp, body = doJSON(t, srv, "GET", "/api/v1/tasks/"+id, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("get status=%d body=%s", resp.StatusCode, body)
	}
}

func TestInvalidCronRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, body := doJSON(t, srv, "POST", "/api/v1/tasks", map[string]interface{}{
		"name":      "bad",
		"kind":      "noop",
		"spec_type": "cron",
		"cron_expr": "99 0 * * *",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, body)
	}
}

func TestTriggerAndRuns(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, body := doJSON(t, srv, "POST", "/api/v1/tasks", map[string]interface{}{
		"name":      "echo-once",
		"kind":      "echo",
		"spec_type": "once",
		"payload":   "hello",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	var created map[string]interface{}
	_ = json.Unmarshal(body, &created)
	id := created["id"].(string)

	resp, _ = doJSON(t, srv, "POST", "/api/v1/tasks/"+id+"/trigger", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("trigger status=%d", resp.StatusCode)
	}
	resp, body = doJSON(t, srv, "GET", "/api/v1/tasks/"+id+"/runs", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("runs status=%d", resp.StatusCode)
	}
	var runs []map[string]interface{}
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0]["status"] != "succeeded" {
		t.Fatalf("run status=%v", runs[0]["status"])
	}
}

func TestHealthAndStats(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, _ := doJSON(t, srv, "GET", "/api/v1/health", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("health=%d", resp.StatusCode)
	}
	resp, body := doJSON(t, srv, "GET", "/api/v1/stats", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stats=%d %s", resp.StatusCode, body)
	}
}
