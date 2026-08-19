package cron_test

import (
	"task110-gosched/internal/model"
	"testing"
	"time"
)

func TestCronUsesDomOrDowWhenBothAreRestrictedAcrossModelAndParser(t *testing.T) {
	loc := time.FixedZone("UTC+14", 14*60*60)
	from := time.Date(2026, 6, 7, 17, 0, 0, 0, loc)
	task := &model.Task{SpecType: model.SpecCron, CronExpr: "0 9 15 * mon"}
	got, err := task.ComputeNextCron(from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 8, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("next=%s want=%s", got, want)
	}
}
