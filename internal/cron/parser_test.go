package cron

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", expr, err)
	}
	return s
}

func TestParseFieldErrors(t *testing.T) {
	cases := []string{
		"* * * *",     // too few
		"* * * * * *", // too many
		"99 * * * *",  // minute out of range
		"* 99 * * *",  // hour out of range
		"* * 0 * *",   // dom out of range
		"* * * 13 *",  // month out of range
		"* * * * 7",   // dow out of range
		"a * * * *",   // non-numeric
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", c)
		}
	}
}

func TestEveryMinute(t *testing.T) {
	s := mustParse(t, "* * * * *")
	base := time.Date(2026, 3, 1, 10, 0, 30, 0, time.UTC)
	got := s.Next(base)
	want := time.Date(2026, 3, 1, 10, 1, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}

func TestSpecificTime(t *testing.T) {
	// At 14:30 every day.
	s := mustParse(t, "30 14 * * *")
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	got := s.Next(base)
	want := time.Date(2026, 3, 1, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}

func TestStepAndList(t *testing.T) {
	// Every 15 minutes, on hours 9 and 17.
	s := mustParse(t, "*/15 9,17 * * *")
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	got := s.Next(base)
	want := time.Date(2026, 3, 1, 9, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}

func TestDayOfWeekName(t *testing.T) {
	// Mondays at midnight.
	s := mustParse(t, "0 0 * * MON")
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // Sunday
	got := s.Next(base)
	want := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}

func TestAdvanceMonth(t *testing.T) {
	// First day of month at 00:00.
	s := mustParse(t, "0 0 1 * *")
	base := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	got := s.Next(base)
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}

func TestRange(t *testing.T) {
	// Minutes 5 through 9.
	s := mustParse(t, "5-9 * * * *")
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got := s.Next(base)
	want := time.Date(2026, 3, 1, 0, 5, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next=%v want %v", got, want)
	}
}
