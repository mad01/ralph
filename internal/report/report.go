package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// Status represents the outcome of a single step.
type Status int

const (
	StatusOK Status = iota
	StatusWarn
	StatusFail
	StatusSkip
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarn:
		return "WARN"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	default:
		return "UNKNOWN"
	}
}

// Verbosity controls how much detail PrintSummary shows.
type Verbosity int

const (
	VerbosityNormal  Verbosity = iota // show fail + warn detail lines
	VerbosityQuiet                    // show only fail detail lines, skip clean phases
	VerbosityVerbose                  // show all items including ok/skip
)

// StepResult records the outcome of one item within a phase.
type StepResult struct {
	Name    string
	Status  Status
	Message string
	Err     error
}

// Phase groups related steps (e.g. "Dotfiles", "Directories").
type Phase struct {
	Name  string
	Steps []StepResult
}

// AddOK records a successful step.
func (p *Phase) AddOK(name, msg string) {
	p.Steps = append(p.Steps, StepResult{Name: name, Status: StatusOK, Message: msg})
}

// AddFail records a failed step.
func (p *Phase) AddFail(name, msg string, err error) {
	p.Steps = append(p.Steps, StepResult{Name: name, Status: StatusFail, Message: msg, Err: err})
}

// AddWarn records a warning step.
func (p *Phase) AddWarn(name, msg string) {
	p.Steps = append(p.Steps, StepResult{Name: name, Status: StatusWarn, Message: msg})
}

// AddSkip records a skipped step.
func (p *Phase) AddSkip(name, msg string) {
	p.Steps = append(p.Steps, StepResult{Name: name, Status: StatusSkip, Message: msg})
}

// Counts returns the number of steps in each status.
func (p *Phase) Counts() (ok, warn, fail, skip int) {
	for _, s := range p.Steps {
		switch s.Status {
		case StatusOK:
			ok++
		case StatusWarn:
			warn++
		case StatusFail:
			fail++
		case StatusSkip:
			skip++
		}
	}
	return
}

// Report collects results across all phases for a command run.
type Report struct {
	Command string
	Phases  []Phase
}

// AddPhase starts tracking a new phase and returns a pointer to it.
func (r *Report) AddPhase(name string) *Phase {
	r.Phases = append(r.Phases, Phase{Name: name})
	return &r.Phases[len(r.Phases)-1]
}

// HasFailures returns true if any step has StatusFail.
func (r *Report) HasFailures() bool {
	for i := range r.Phases {
		for _, s := range r.Phases[i].Steps {
			if s.Status == StatusFail {
				return true
			}
		}
	}
	return false
}

// HasWarnings returns true if any step has StatusWarn.
func (r *Report) HasWarnings() bool {
	for i := range r.Phases {
		for _, s := range r.Phases[i].Steps {
			if s.Status == StatusWarn {
				return true
			}
		}
	}
	return false
}

// ExitCode returns 0 for clean, 1 for failures, 2 for warnings-only.
func (r *Report) ExitCode() int {
	if r.HasFailures() {
		return 1
	}
	if r.HasWarnings() {
		return 2
	}
	return 0
}

// PrintSummary writes the end-of-run summary to w.
func (r *Report) PrintSummary(w io.Writer, v Verbosity) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "--- Summary ---")
	fmt.Fprintln(w)

	totalOK, totalWarn, totalFail, totalSkip := 0, 0, 0, 0

	for i := range r.Phases {
		p := &r.Phases[i]
		ok, warn, fail, skip := p.Counts()
		totalOK += ok
		totalWarn += warn
		totalFail += fail
		totalSkip += skip

		// In quiet mode, skip phases with no failures.
		if v == VerbosityQuiet && fail == 0 {
			continue
		}

		// Print phase count line.
		fmt.Fprintf(w, "%s: %s\n", p.Name, formatCounts(ok, warn, fail, skip))

		// Print detail lines based on verbosity.
		for _, s := range p.Steps {
			switch {
			case s.Status == StatusFail:
				fmt.Fprintf(w, "  %s %s: %s\n", color.RedString("FAIL"), s.Name, s.Message)
			case s.Status == StatusWarn && v != VerbosityQuiet:
				fmt.Fprintf(w, "  %s %s: %s\n", color.YellowString("WARN"), s.Name, s.Message)
			case v == VerbosityVerbose && s.Status == StatusOK:
				fmt.Fprintf(w, "  %s %s\n", color.GreenString("OK"), s.Name)
			case v == VerbosityVerbose && s.Status == StatusSkip:
				fmt.Fprintf(w, "  %s %s: %s\n", color.CyanString("SKIP"), s.Name, s.Message)
			}
		}
	}

	// Totals line.
	fmt.Fprintln(w)
	parts := []string{color.GreenString("%d ok", totalOK)}
	if totalWarn > 0 {
		parts = append(parts, color.YellowString("%d warnings", totalWarn))
	}
	if totalFail > 0 {
		parts = append(parts, color.RedString("%d failed", totalFail))
	}
	if totalSkip > 0 {
		parts = append(parts, color.CyanString("%d skipped", totalSkip))
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))

	if r.HasFailures() {
		color.New(color.FgRed).Fprintln(w, "Some items failed. Review the details above.")
	} else if r.HasWarnings() {
		color.New(color.FgYellow).Fprintln(w, "Completed with warnings.")
	}
}

// formatCounts builds a compact "N ok, N warn, N fail, N skip" string,
// omitting zero-value categories.
func formatCounts(ok, warn, fail, skip int) string {
	var parts []string
	if ok > 0 {
		parts = append(parts, color.GreenString("%d ok", ok))
	}
	if warn > 0 {
		parts = append(parts, color.YellowString("%d warn", warn))
	}
	if fail > 0 {
		parts = append(parts, color.RedString("%d fail", fail))
	}
	if skip > 0 {
		parts = append(parts, color.CyanString("%d skip", skip))
	}
	if len(parts) == 0 {
		return "nothing to report"
	}
	return strings.Join(parts, ", ")
}
