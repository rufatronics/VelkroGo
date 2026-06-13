package scheduler

import (
	"testing"
	"time"
)

func TestNextCronEvery15Min(t *testing.T) {
	from := time.Date(2026, 1, 1, 9, 7, 0, 0, time.UTC)
	next, err := nextCron("*/15 * * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	// Expect 09:15.
	if next.Minute() != 15 || next.Hour() != 9 {
		t.Fatalf("expected 09:15, got %s", next.Format("15:04"))
	}
}

func TestNextRunOnce(t *testing.T) {
	future := time.Now().Add(2 * time.Hour)
	s := future.UTC().Format(time.RFC3339)
	next, err := nextRunTime("once:"+s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if next == nil {
		t.Fatal("expected non-nil next for future once job")
	}
	if next.Unix() != future.Unix() {
		t.Fatalf("time mismatch: got %v want %v", next.UTC(), future.UTC())
	}
}

func TestNextRunOncePast(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	s := past.UTC().Format(time.RFC3339)
	next, err := nextRunTime("once:"+s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatal("past once job should return nil next")
	}
}

func TestEveryDuration(t *testing.T) {
	from := time.Now()
	next, err := nextRunTime("every 30m", from)
	if err != nil {
		t.Fatal(err)
	}
	diff := next.Sub(from)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Fatalf("expected ~30m, got %v", diff)
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		s    string
		want time.Duration
	}{
		{"15m", 15 * time.Minute},
		{"2h", 2 * time.Hour},
		{"30 min", 30 * time.Minute},
		{"1 day", 24 * time.Hour},
	}
	for _, c := range cases {
		d, err := parseDuration(c.s)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", c.s, err)
			continue
		}
		if d != c.want {
			t.Errorf("parseDuration(%q) = %v, want %v", c.s, d, c.want)
		}
	}
}
