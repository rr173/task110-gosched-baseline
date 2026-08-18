package clock

import (
	"testing"
	"time"
)

func TestFakeClockNowAndAdd(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewFakeClock(base)
	if !c.Now().Equal(base) {
		t.Fatalf("Now mismatch: got %v want %v", c.Now(), base)
	}
	got := c.Add(2 * time.Hour)
	want := base.Add(2 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("Add mismatch: got %v want %v", got, want)
	}
	if !c.Now().Equal(want) {
		t.Fatalf("Now after Add mismatch: got %v want %v", c.Now(), want)
	}
}

func TestFakeClockSet(t *testing.T) {
	c := NewFakeClock(time.Unix(0, 0))
	c.Set(time.Unix(1000, 0))
	if c.Now().Unix() != 1000 {
		t.Fatalf("Set failed: got %d", c.Now().Unix())
	}
}

func TestRealClockNow(t *testing.T) {
	var c Clock = RealClock{}
	if c.Now().IsZero() {
		t.Fatal("RealClock.Now returned zero time")
	}
}
