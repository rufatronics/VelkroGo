// Package scheduler runs one-shot and recurring jobs even when no UI is open.
// Natural-language schedule strings ("once:2026-06-20T09:00:00Z", cron
// expressions like "*/15 * * * *", or "every 15m") are stored in the DB; the
// Scheduler ticks every minute and dispatches due jobs to a user-supplied
// handler.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/rufatronics/velkrogo/internal/memory"
)

// Handler is called when a job is due. The implementation runs an agent turn.
type Handler func(ctx context.Context, job memory.Job)

// Scheduler ticks every minute and fires due jobs.
type Scheduler struct {
	db      *memory.DB
	handler Handler
	mu      sync.Mutex
	cancel  context.CancelFunc
}

// New creates a Scheduler backed by db. Call Start to begin ticking.
func New(db *memory.DB, h Handler) *Scheduler {
	return &Scheduler{db: db, handler: h}
}

// Start begins the scheduler loop in the background.
func (s *Scheduler) Start(ctx context.Context) {
	ctx2, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	go s.loop(ctx2)
}

// Stop halts the scheduler loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *Scheduler) loop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	// Check immediately on start too.
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	jobs, err := s.db.ListJobs()
	if err != nil {
		log.Printf("scheduler: list jobs: %v", err)
		return
	}
	now := time.Now().UTC()
	for _, j := range jobs {
		if !j.Enabled {
			continue
		}
		if j.NextRun == nil || now.Before(*j.NextRun) {
			continue
		}
		// Fire.
		go s.handler(ctx, j)
		// Update LastRun and compute NextRun.
		lr := now
		j.LastRun = &lr
		next, err := nextRunTime(j.Schedule, now)
		if err != nil {
			log.Printf("scheduler: job %s: bad schedule %q: %v", j.ID, j.Schedule, err)
			// Disable the job to prevent spam.
			j.Enabled = false
			j.NextRun = nil
		} else {
			j.NextRun = next
		}
		if err := s.db.UpsertJob(j); err != nil {
			log.Printf("scheduler: update job %s: %v", j.ID, err)
		}
	}
}

// AddJob creates or replaces a job and computes its first NextRun.
func (s *Scheduler) AddJob(j memory.Job) error {
	now := time.Now().UTC()
	next, err := nextRunTime(j.Schedule, now)
	if err != nil {
		return fmt.Errorf("invalid schedule %q: %w", j.Schedule, err)
	}
	j.NextRun = next
	j.Enabled = true
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	return s.db.UpsertJob(j)
}

// nextRunTime computes the next execution time from schedule string and current time.
// Supported formats:
//   - "once:RFC3339"     e.g. "once:2026-06-20T09:00:00Z"
//   - "every Xm/Xh/Xd"  e.g. "every 15m", "every 2h"
//   - 5-field cron       e.g. "*/15 * * * *"
func nextRunTime(schedule string, from time.Time) (*time.Time, error) {
	// One-shot.
	if strings.HasPrefix(schedule, "once:") {
		t, err := time.Parse(time.RFC3339, strings.TrimPrefix(schedule, "once:"))
		if err != nil {
			return nil, fmt.Errorf("parse once time: %w", err)
		}
		if t.Before(from) {
			return nil, nil // already passed; disable
		}
		return &t, nil
	}
	// "every Xm / Xh / Xd"
	if strings.HasPrefix(strings.ToLower(schedule), "every ") {
		dur, err := parseDuration(strings.TrimPrefix(strings.ToLower(schedule), "every "))
		if err != nil {
			return nil, err
		}
		t := from.Add(dur)
		return &t, nil
	}
	// Minimal 5-field cron.
	t, err := nextCron(schedule, from)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	// Try standard Go duration first ("15m", "2h30m", etc.)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// "15 min" / "2 hours" / "1 day"
	parts := strings.Fields(s)
	if len(parts) == 2 {
		var n int
		if _, err := fmt.Sscanf(parts[0], "%d", &n); err != nil {
			return 0, fmt.Errorf("can't parse %q as duration", s)
		}
		switch strings.TrimSuffix(strings.ToLower(parts[1]), "s") {
		case "second", "sec":
			return time.Duration(n) * time.Second, nil
		case "minute", "min":
			return time.Duration(n) * time.Minute, nil
		case "hour", "hr":
			return time.Duration(n) * time.Hour, nil
		case "day":
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, fmt.Errorf("can't parse %q as duration", s)
}

// nextCron is a minimal cron evaluator for the 5-field minute-resolution
// format: min hour dom month dow. We support * and */n; no ranges for now.
func nextCron(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 cron fields, got %d", len(fields))
	}
	t := from.Add(time.Minute).Truncate(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if matchCron(fields, t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no matching time found within a year")
}

func matchCron(fields []string, t time.Time) bool {
	return cronMatch(fields[0], t.Minute()) &&
		cronMatch(fields[1], t.Hour()) &&
		cronMatch(fields[2], t.Day()) &&
		cronMatch(fields[3], int(t.Month())) &&
		cronMatch(fields[4], int(t.Weekday()))
}

func cronMatch(field string, val int) bool {
	if field == "*" {
		return true
	}
	if strings.HasPrefix(field, "*/") {
		var step int
		fmt.Sscanf(field[2:], "%d", &step)
		return step > 0 && val%step == 0
	}
	var n int
	fmt.Sscanf(field, "%d", &n)
	return n == val
}
