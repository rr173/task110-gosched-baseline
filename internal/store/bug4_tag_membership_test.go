package store

import "testing"

func TestTagMembershipIsConsistentAcrossQueriesAndBulkMutation(t *testing.T) {
	s := openTest(t)
	tx, _ := s.BeginTx()
	task := newTask("tagged")
	task.Tags = []string{"team", "ops"}
	if err := s.CreateTask(tx, task); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, _ = s.BeginTx()
	listed, err := s.TasksByTag(tx, "ops")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("tag query found %d tasks", len(listed))
	}
	if _, err := s.BulkPauseByTag(tx, "ops", 2); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask(tx, task.ID)
	_ = tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Fatalf("bulk tag mutation disagrees with tag query: %+v", got)
	}
}
