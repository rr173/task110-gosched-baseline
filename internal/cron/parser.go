// Package cron implements a small but real 5-field cron expression parser
// (minute hour day-of-month month day-of-week) with deterministic Next()
// computation. It supports "*", "*/step", "a-b", "a-b/step" and comma lists,
// plus 3-letter month/day names.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxYears limits how far Next() will search for a match.
const maxYears = 5

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var dowNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// Schedule is a parsed cron expression.
type Schedule struct {
	minute map[int]bool
	hour   map[int]bool
	dom    map[int]bool
	month  map[int]bool
	dow    map[int]bool
}

// Parse parses a 5-field cron string.
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}
	s := &Schedule{
		minute: make(map[int]bool),
		hour:   make(map[int]bool),
		dom:    make(map[int]bool),
		month:  make(map[int]bool),
		dow:    make(map[int]bool),
	}
	if err := parseField(fields[0], 0, 59, nil, s.minute); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if err := parseField(fields[1], 0, 23, nil, s.hour); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if err := parseField(fields[2], 1, 31, nil, s.dom); err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	if err := parseField(fields[3], 1, 12, monthNames, s.month); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	if err := parseField(fields[4], 0, 6, dowNames, s.dow); err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	return s, nil
}

// parseField fills set from one cron field spec.
func parseField(spec string, min, max int, names map[string]int, set map[int]bool) error {
	if spec == "" {
		return fmt.Errorf("empty field")
	}
	for _, part := range strings.Split(spec, ",") {
		if err := parsePart(part, min, max, names, set); err != nil {
			return err
		}
	}
	if len(set) == 0 {
		return fmt.Errorf("field %q matched nothing", spec)
	}
	return nil
}

func parsePart(part string, min, max int, names map[string]int, set map[int]bool) error {
	step := 1
	rangeStr := part
	if idx := strings.Index(part, "/"); idx >= 0 {
		stepStr := part[idx+1:]
		st, err := strconv.Atoi(stepStr)
		if err != nil || st <= 0 {
			return fmt.Errorf("bad step %q", stepStr)
		}
		step = st
		rangeStr = part[:idx]
	}

	lo, hi := min, max
	if rangeStr == "*" {
		// lo/hi already set to bounds
	} else if strings.Contains(rangeStr, "-") {
		bounds := strings.SplitN(rangeStr, "-", 2)
		a, err := resolve(bounds[0], names)
		if err != nil {
			return err
		}
		b, err := resolve(bounds[1], names)
		if err != nil {
			return err
		}
		lo, hi = a, b
	} else {
		v, err := resolve(rangeStr, names)
		if err != nil {
			return err
		}
		lo, hi = v, v
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("value %d-%d out of range [%d,%d]", lo, hi, min, max)
	}
	for v := lo; v <= hi; v += step {
		set[v] = true
	}
	return nil
}

func resolve(tok string, names map[string]int) (int, error) {
	if names != nil {
		if v, ok := names[strings.ToLower(tok)]; ok {
			return v, nil
		}
	}
	return strconv.Atoi(tok)
}

// Next returns the next time strictly after `from` that matches the schedule.
// The returned time is truncated to the minute. It returns a zero time if no
// match is found within maxYears.
func (s *Schedule) Next(from time.Time) time.Time {
	t := from.Add(time.Minute).Truncate(time.Minute)
	limit := t.AddDate(maxYears, 0, 0)
	for t.Before(limit) {
		if !s.month[int(t.Month())] {
			t = firstOfNextMonth(t)
			continue
		}
		if !s.dom[t.Day()] {
			t = nextDay(t)
			continue
		}
		if !s.dow[int(t.Weekday())] {
			t = nextDay(t)
			continue
		}
		if !s.hour[t.Hour()] {
			t = nextHour(t)
			continue
		}
		if !s.minute[t.Minute()] {
			t = t.Add(time.Minute).Truncate(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

func firstOfNextMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	nm := time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
	return nm
}

func nextDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

func nextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).
		Add(time.Hour)
}
