// Package clock abstracts time sources so the scheduler can be tested with a
// fake clock that advances deterministically without real sleeps.
package clock

import (
	"sync"
	"time"
)

// Clock abstracts the time operations the scheduler relies on.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	Sleep(d time.Duration)
}

// RealClock uses the system wall clock.
type RealClock struct{}

func (RealClock) Now() time.Time                         { return time.Now() }
func (RealClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (RealClock) Sleep(d time.Duration)                  { time.Sleep(d) }

// FakeClock is a manual clock. Now returns the current fake time; Add advances
// it. After/Sleep are best-effort and primarily exist to satisfy the interface;
// scheduler tests drive time via Add + explicit Tick calls, not via sleeps.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock creates a fake clock at the given time.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

// Now returns the current fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Add advances the fake clock by d and returns the new time.
func (c *FakeClock) Add(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

// Set replaces the fake clock time.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// After is a no-op channel that never fires for the fake clock; scheduler tests
// do not rely on wall-clock timers.
func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	return make(chan time.Time)
}

// Sleep does nothing for the fake clock.
func (c *FakeClock) Sleep(d time.Duration) {}
