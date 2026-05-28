package progress

import (
	"testing"
	"time"
)

// Smoke test: the counter lifecycle must not panic in a non-TTY environment
// (the test runner), and the silent counter must be a no-op.
func TestCounter_LifecycleDoesNotPanic(t *testing.T) {
	c := New("building", 3)
	c.Tick()
	c.TickWith("item-one")
	c.TickWith("a-very-long-item-name-that-exceeds-the-available-width-budget")
	c.Done()

	q := NewQuiet()
	q.Tick()
	q.TickWith("x")
	q.Done()

	StatusLine("ok phase", true)
	StatusLine("failed phase", false)
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		500 * time.Millisecond:  "0.5s",
		1500 * time.Millisecond: "1.5s",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
	// Minute-plus durations should include a minute component (format-agnostic).
	if got := formatDuration(90 * time.Second); got == "" {
		t.Error("formatDuration(90s) returned empty")
	}
}
