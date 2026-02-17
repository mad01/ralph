package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

func init() {
	// Disable color output for deterministic test assertions.
	color.NoColor = true
}

func TestPhaseCounts(t *testing.T) {
	tests := []struct {
		name                   string
		steps                  []StepResult
		wantOK, wantWarn, wantFail, wantSkip int
	}{
		{
			name:     "empty phase",
			steps:    nil,
			wantOK:   0, wantWarn: 0, wantFail: 0, wantSkip: 0,
		},
		{
			name: "all ok",
			steps: []StepResult{
				{Name: "a", Status: StatusOK},
				{Name: "b", Status: StatusOK},
			},
			wantOK: 2, wantWarn: 0, wantFail: 0, wantSkip: 0,
		},
		{
			name: "mixed statuses",
			steps: []StepResult{
				{Name: "a", Status: StatusOK},
				{Name: "b", Status: StatusWarn, Message: "something"},
				{Name: "c", Status: StatusFail, Message: "broken"},
				{Name: "d", Status: StatusSkip, Message: "disabled"},
				{Name: "e", Status: StatusOK},
				{Name: "f", Status: StatusFail, Message: "also broken"},
			},
			wantOK: 2, wantWarn: 1, wantFail: 2, wantSkip: 1,
		},
		{
			name: "only skips",
			steps: []StepResult{
				{Name: "a", Status: StatusSkip},
				{Name: "b", Status: StatusSkip},
			},
			wantOK: 0, wantWarn: 0, wantFail: 0, wantSkip: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Phase{Name: "test", Steps: tt.steps}
			ok, warn, fail, skip := p.Counts()
			if ok != tt.wantOK || warn != tt.wantWarn || fail != tt.wantFail || skip != tt.wantSkip {
				t.Errorf("Counts() = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
					ok, warn, fail, skip, tt.wantOK, tt.wantWarn, tt.wantFail, tt.wantSkip)
			}
		})
	}
}

func TestHasFailures(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   bool
	}{
		{
			name:   "empty report",
			phases: nil,
			want:   false,
		},
		{
			name: "all ok",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusOK}}},
			},
			want: false,
		},
		{
			name: "warnings only",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusWarn}}},
			},
			want: false,
		},
		{
			name: "has failure",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusOK}}},
				{Name: "p2", Steps: []StepResult{{Status: StatusFail}}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Report{Phases: tt.phases}
			if got := r.HasFailures(); got != tt.want {
				t.Errorf("HasFailures() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasWarnings(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   bool
	}{
		{
			name:   "empty report",
			phases: nil,
			want:   false,
		},
		{
			name: "all ok",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusOK}}},
			},
			want: false,
		},
		{
			name: "has warning",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusWarn, Message: "hmm"}}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Report{Phases: tt.phases}
			if got := r.HasWarnings(); got != tt.want {
				t.Errorf("HasWarnings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   int
	}{
		{
			name:   "empty is clean",
			phases: nil,
			want:   0,
		},
		{
			name: "all ok is clean",
			phases: []Phase{
				{Name: "p", Steps: []StepResult{{Status: StatusOK}, {Status: StatusSkip}}},
			},
			want: 0,
		},
		{
			name: "warnings only returns 2",
			phases: []Phase{
				{Name: "p", Steps: []StepResult{{Status: StatusOK}, {Status: StatusWarn}}},
			},
			want: 2,
		},
		{
			name: "failures return 1",
			phases: []Phase{
				{Name: "p", Steps: []StepResult{{Status: StatusFail}}},
			},
			want: 1,
		},
		{
			name: "failures take precedence over warnings",
			phases: []Phase{
				{Name: "p1", Steps: []StepResult{{Status: StatusWarn}}},
				{Name: "p2", Steps: []StepResult{{Status: StatusFail}}},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Report{Phases: tt.phases}
			if got := r.ExitCode(); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAddPhaseAndHelpers(t *testing.T) {
	r := &Report{Command: "test"}
	p := r.AddPhase("Phase1")
	p.AddOK("step1", "all good")
	p.AddFail("step2", "broken", nil)
	p.AddWarn("step3", "hmm")
	p.AddSkip("step4", "disabled")

	if len(r.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(r.Phases))
	}
	if len(r.Phases[0].Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(r.Phases[0].Steps))
	}

	ok, warn, fail, skip := p.Counts()
	if ok != 1 || warn != 1 || fail != 1 || skip != 1 {
		t.Errorf("Counts() = (%d, %d, %d, %d), want (1, 1, 1, 1)", ok, warn, fail, skip)
	}
}

func TestPrintSummaryNormal(t *testing.T) {
	r := buildTestReport()
	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityNormal)
	out := buf.String()

	assertContains(t, out, "--- Summary ---")
	assertContains(t, out, "Dotfiles:")
	assertContains(t, out, "FAIL broken_link")
	assertContains(t, out, "WARN gitconfig")
	// Normal mode should not show OK detail lines
	if strings.Contains(out, "OK vimrc") {
		t.Error("Normal verbosity should not show OK detail lines")
	}
	// Should contain totals
	assertContains(t, out, "ok")
	assertContains(t, out, "failed")
}

func TestPrintSummaryQuiet(t *testing.T) {
	r := buildTestReport()
	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityQuiet)
	out := buf.String()

	assertContains(t, out, "--- Summary ---")
	assertContains(t, out, "FAIL broken_link")
	// Quiet mode should not show WARN detail lines
	if strings.Contains(out, "WARN gitconfig") {
		t.Error("Quiet verbosity should not show WARN detail lines")
	}
	// Quiet mode should skip clean phases (Tools has no failures)
	if strings.Contains(out, "Tools:") {
		t.Error("Quiet verbosity should skip phases with no failures")
	}
}

func TestPrintSummaryVerbose(t *testing.T) {
	r := buildTestReport()
	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityVerbose)
	out := buf.String()

	assertContains(t, out, "--- Summary ---")
	assertContains(t, out, "OK vimrc")
	assertContains(t, out, "FAIL broken_link")
	assertContains(t, out, "WARN gitconfig")
	assertContains(t, out, "SKIP tmux")
	assertContains(t, out, "Tools:")
}

func TestPrintSummaryFailureMessage(t *testing.T) {
	r := &Report{Command: "test"}
	p := r.AddPhase("Items")
	p.AddFail("x", "something broke", nil)

	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityNormal)
	assertContains(t, buf.String(), "Some items failed.")
}

func TestPrintSummaryWarningMessage(t *testing.T) {
	r := &Report{Command: "test"}
	p := r.AddPhase("Items")
	p.AddOK("a", "")
	p.AddWarn("b", "not great")

	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityNormal)
	assertContains(t, buf.String(), "Completed with warnings.")
}

func TestPrintSummaryClean(t *testing.T) {
	r := &Report{Command: "test"}
	p := r.AddPhase("Items")
	p.AddOK("a", "")
	p.AddOK("b", "")

	var buf bytes.Buffer
	r.PrintSummary(&buf, VerbosityNormal)
	out := buf.String()
	if strings.Contains(out, "failed") || strings.Contains(out, "warnings") {
		t.Error("Clean report should not mention failures or warnings in totals")
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		s    Status
		want string
	}{
		{StatusOK, "OK"},
		{StatusWarn, "WARN"},
		{StatusFail, "FAIL"},
		{StatusSkip, "SKIP"},
		{Status(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

// buildTestReport creates a report with mixed outcomes for testing PrintSummary.
func buildTestReport() *Report {
	r := &Report{Command: "test"}

	df := r.AddPhase("Dotfiles")
	df.AddOK("vimrc", "")
	df.AddOK("bashrc", "")
	df.AddFail("broken_link", "source file does not exist", nil)
	df.AddWarn("gitconfig", "post-hook exit status 1")
	df.AddSkip("tmux", "disabled")

	tools := r.AddPhase("Tools")
	tools.AddOK("git", "installed")
	tools.AddWarn("ripgrep", "not installed")

	return r
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output does not contain %q\n--- output ---\n%s", needle, haystack)
	}
}
