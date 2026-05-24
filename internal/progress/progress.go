package progress

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

var isTTY = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

// Counter tracks progress through a set of items and renders a single-line
// counter on stdout. On a TTY, it overwrites in place with \r. When not a
// TTY (piped output), it prints the final line only.
// If silent is true, the counter produces no output (for verbose mode where
// per-item detail is already shown).
type Counter struct {
	label  string
	total  int
	cur    int
	silent bool
}

// New creates a counter with a label (e.g. "Dotfiles") and total item count.
func New(label string, total int) *Counter {
	c := &Counter{label: label, total: total}
	if isTTY && total > 0 {
		fmt.Fprintf(os.Stdout, "  %s [0/%d]", label, total)
	}
	return c
}

// NewQuiet creates a counter that is silent (for verbose/dry-run modes where
// per-item detail is already shown).
func NewQuiet() *Counter {
	return &Counter{silent: true}
}

// Tick advances the counter by one and updates the display.
func (c *Counter) Tick() {
	if c.silent {
		return
	}
	c.cur++
	if isTTY {
		line := fmt.Sprintf("  %s [%d/%d]", c.label, c.cur, c.total)
		pad := 40 - len(line)
		if pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		fmt.Fprintf(os.Stdout, "\r%s", line)
	}
}

// Done completes the counter, printing the final state with a newline.
func (c *Counter) Done() {
	if c.silent {
		return
	}
	if isTTY {
		line := fmt.Sprintf("  %s [%d/%d]", c.label, c.cur, c.total)
		pad := 40 - len(line)
		if pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		fmt.Fprintf(os.Stdout, "\r%s\n", line)
	} else {
		fmt.Fprintf(os.Stdout, "  %s [%d/%d]\n", c.label, c.cur, c.total)
	}
}
