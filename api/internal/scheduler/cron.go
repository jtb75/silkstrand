// Package scheduler provides an in-process scan-definition scheduler
// per ADR 007 D4. A single goroutine ticks every 30 seconds, claims
// due definitions via `SELECT ... FOR UPDATE SKIP LOCKED`, advances
// `next_run_at`, and dispatches each one through the same code path
// used by manual execution.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron holds a parsed 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week.
// Accepted tokens per field: "*", "N", "N-M", "N,M,...", "*/S", "N-M/S".
// Day-of-week: 0-6 (Sunday=0; we also accept 7 as Sunday for robustness).
// No seconds, no named month/dow, no "L"/"W"/"#" extensions — keep it
// boring. If a definition needs richer cron later we pull in
// robfig/cron/v3 (single-commit dep swap).
type Cron struct {
	Minute [60]bool
	Hour   [24]bool
	Dom    [32]bool // 1..31
	Month  [13]bool // 1..12
	Dow    [7]bool  // 0..6
}

// ParseCron parses a 5-field cron expression.
func ParseCron(expr string) (*Cron, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d (%q)", len(fields), expr)
	}
	c := &Cron{}
	if err := parseField(fields[0], 0, 59, c.Minute[:]); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if err := parseField(fields[1], 0, 23, c.Hour[:]); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if err := parseField(fields[2], 1, 31, c.Dom[:]); err != nil {
		return nil, fmt.Errorf("cron dom: %w", err)
	}
	if err := parseField(fields[3], 1, 12, c.Month[:]); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	if err := parseField(fields[4], 0, 7, c.Dow[:]); err != nil {
		return nil, fmt.Errorf("cron dow: %w", err)
	}
	// Fold 7 -> 0 (Sunday). c.Dow is fixed-size [7]bool; the parser was
	// passed a slice of len=7 so index 7 never occurs — handled there via
	// max=7 with post-fold in parseField.
	return c, nil
}

func parseField(expr string, min, max int, out []bool) error {
	for _, part := range strings.Split(expr, ",") {
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			s, err := strconv.Atoi(part[idx+1:])
			if err != nil || s <= 0 {
				return fmt.Errorf("invalid step %q", part)
			}
			step = s
			part = part[:idx]
		}
		lo, hi := min, max
		if part != "*" && part != "" {
			if idx := strings.Index(part, "-"); idx >= 0 {
				a, err := strconv.Atoi(part[:idx])
				if err != nil {
					return fmt.Errorf("invalid range start %q", part)
				}
				b, err := strconv.Atoi(part[idx+1:])
				if err != nil {
					return fmt.Errorf("invalid range end %q", part)
				}
				lo, hi = a, b
			} else {
				n, err := strconv.Atoi(part)
				if err != nil {
					return fmt.Errorf("invalid number %q", part)
				}
				lo, hi = n, n
			}
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("out of range [%d,%d]: %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			// Fold dow=7 -> 0.
			idx := v
			if len(out) == 7 && idx == 7 {
				idx = 0
			}
			if idx < len(out) {
				out[idx] = true
			}
		}
	}
	return nil
}

// Next returns the smallest time strictly after `from` that matches
// this cron expression. Truncated to minute resolution.
func (c *Cron) Next(from time.Time) (time.Time, error) {
	// Start at the minute after `from`.
	t := from.Add(time.Minute).Truncate(time.Minute)
	// Safety cap: scan up to 4 years of candidate minutes — more than
	// enough for any reasonable pattern, bounded so a bogus expression
	// can't loop forever.
	for i := 0; i < 366*4*24*60; i++ {
		if c.matches(t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron: no match within 4 years")
}

func (c *Cron) matches(t time.Time) bool {
	return c.Minute[t.Minute()] &&
		c.Hour[t.Hour()] &&
		c.Dom[t.Day()] &&
		c.Month[int(t.Month())] &&
		c.Dow[int(t.Weekday())]
}
