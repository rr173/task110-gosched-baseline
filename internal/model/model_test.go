package model

import (
	"testing"
	"time"
)

func TestValidateCron(t *testing.T) {
	tk := &Task{Name: "t", Kind: KindNoop, SpecType: SpecCron, CronExpr: "0 0 * * *"}
	if err := tk.Validate(); err != nil {
		t.Fatalf("valid cron rejected: %v", err)
	}
	bad := &Task{Name: "t", Kind: KindNoop, SpecType: SpecCron, CronExpr: "99 0 * * *"}
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid cron expr accepted")
	}
}

func TestValidateDelayed(t *testing.T) {
	bad := &Task{Name: "t", Kind: KindNoop, SpecType: SpecDelayed, DelaySeconds: 0}
	if err := bad.Validate(); err == nil {
		t.Fatal("delayed with zero delay accepted")
	}
	good := &Task{Name: "t", Kind: KindNoop, SpecType: SpecDelayed, DelaySeconds: 30}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid delayed rejected: %v", err)
	}
}

func TestValidateKindAndName(t *testing.T) {
	if err := (&Task{Kind: KindNoop, SpecType: SpecOnce}).Validate(); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := (&Task{Name: "t", Kind: "bogus", SpecType: SpecOnce}).Validate(); err == nil {
		t.Fatal("bogus kind accepted")
	}
}

func TestStatusDerived(t *testing.T) {
	disabled := &Task{Enabled: false}
	if disabled.Status() != "disabled" {
		t.Fatalf("got %s", disabled.Status())
	}
	paused := &Task{Enabled: true, Paused: true}
	if paused.Status() != "paused" {
		t.Fatalf("got %s", paused.Status())
	}
	active := &Task{Enabled: true}
	if active.Status() != "active" {
		t.Fatalf("got %s", active.Status())
	}
}

func TestIsDue(t *testing.T) {
	now := time.Now().UnixMilli()
	due := &Task{Enabled: true, NextRunAt: now - 1000}
	if !due.IsDue(now) {
		t.Fatal("expected due")
	}
	future := &Task{Enabled: true, NextRunAt: now + 1000}
	if future.IsDue(now) {
		t.Fatal("future should not be due")
	}
	paused := &Task{Enabled: true, Paused: true, NextRunAt: now - 1000}
	if paused.IsDue(now) {
		t.Fatal("paused should not be due")
	}
}

func TestComputeNextCron(t *testing.T) {
	tk := &Task{SpecType: SpecCron, CronExpr: "0 12 * * *"}
	from := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	next, err := tk.ComputeNextCron(from)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Equal(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v", next)
	}
}

func TestTagsCSV(t *testing.T) {
	if got := TagsCSV([]string{"  b ", "a", "", "a"}); got != "b,a" {
		t.Fatalf("got %q", got)
	}
}
