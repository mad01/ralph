package progress

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

var isTTY = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

const clearPad = 20

// Counter tracks progress through a set of items and renders a single-line
// counter on stdout. On a TTY, it overwrites in place with \r and shows a
// completion line with a checkmark and elapsed time.
type Counter struct {
	label  string
	total  int
	cur    int
	silent bool
	start  time.Time
}

// New creates a counter with a label and total item count.
func New(label string, total int) *Counter {
	c := &Counter{label: label, total: total, start: time.Now()}
	if isTTY && total > 0 {
		fmt.Fprintf(os.Stdout, "  %s [0/%d]%s", label, total, strings.Repeat(" ", clearPad))
	}
	return c
}

// NewQuiet creates a silent counter for verbose/dry-run modes.
func NewQuiet() *Counter {
	return &Counter{silent: true}
}

// Tick advances the counter by one.
func (c *Counter) Tick() { c.TickWith("") }

// TickWith advances the counter and shows the current item name.
func (c *Counter) TickWith(item string) {
	if c.silent {
		return
	}
	c.cur++
	if isTTY {
		line := fmt.Sprintf("  %s [%d/%d]", c.label, c.cur, c.total)
		if item != "" {
			avail := 50 - len(line)
			if avail > 3 {
				if len(item) > avail {
					item = item[:avail-1] + "…"
				}
				line += " " + item
			}
		}
		fmt.Fprintf(os.Stdout, "\r%s%s", line, strings.Repeat(" ", clearPad))
	}
}

// Done completes the counter with a checkmark and optional elapsed time.
func (c *Counter) Done() {
	if c.silent {
		return
	}
	elapsed := time.Since(c.start)
	if isTTY {
		line := fmt.Sprintf("  %s %s [%d/%d]", color.GreenString("✓"), c.label, c.cur, c.total)
		if elapsed >= time.Second {
			line += "  " + color.HiBlackString(formatDuration(elapsed))
		}
		fmt.Fprintf(os.Stdout, "\r%s%s\n", line, strings.Repeat(" ", clearPad))
	} else {
		fmt.Fprintf(os.Stdout, "  %s [%d/%d]\n", c.label, c.cur, c.total)
	}
}

// StatusLine prints a single status line for phases without a counter.
func StatusLine(label string, ok bool) {
	if !isTTY {
		return
	}
	sym := color.GreenString("✓")
	if !ok {
		sym = color.RedString("✗")
	}
	fmt.Fprintf(os.Stdout, "  %s %s\n", sym, label)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := d.Seconds() - float64(m)*60
	return fmt.Sprintf("%dm%.0fs", m, s)
}
