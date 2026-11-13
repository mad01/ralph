package progress

import (
	"os"
	"strings"
	"testing"
	"time"
)

// onTerminal runs fn with stdout treated as a terminal and returns what fn
// wrote to stdout. It restores the cursor state, so a counter fn leaves open
// does not leak into the next test.
func onTerminal(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	savedTTY, savedOut := isTTY, os.Stdout
	isTTY, os.Stdout = true, f
	func() {
		defer func() { isTTY, os.Stdout = savedTTY, savedOut }()
		fn()
	}()
	t.Cleanup(func() { cursorHidden.Store(false) })
	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// Verbose and dry-run modes print their own per-item lines: on a terminal a
// quiet counter must write nothing, the cursor escape codes included, and
// must not leave the cursor marked hidden.
func TestNewOrQuiet_QuietWritesNothingOnTerminal(t *testing.T) {
	out := onTerminal(t, func() {
		c := NewOrQuiet("Dotfiles", 3, true)
		c.TickWith("zshrc")
		c.Done()
	})
	if out != "" {
		t.Errorf("quiet counter wrote %q, want nothing", out)
	}
	if cursorHidden.Load() {
		t.Error("quiet counter left the cursor marked hidden")
	}
}

// A live counter from NewOrQuiet writes exactly what New writes.
func TestNewOrQuiet_LiveMatchesNew(t *testing.T) {
	run := func(newCounter func() *Counter) string {
		return onTerminal(t, func() {
			c := newCounter()
			c.TickWith("zshrc")
		})
	}
	got := run(func() *Counter { return NewOrQuiet("Dotfiles", 3, false) })
	want := run(func() *Counter { return New("Dotfiles", 3) })
	if got != want {
		t.Errorf("NewOrQuiet live wrote %q, New wrote %q", got, want)
	}
	if !strings.HasPrefix(got, "\033[?25l") {
		t.Errorf("live counter output %q does not hide the cursor first", got)
	}
}

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
