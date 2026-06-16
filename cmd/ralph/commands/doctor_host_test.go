package commands

import (
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// findStep returns the StepResult for name in the named phase, or nil.
func findStep(rpt *report.Report, phase, name string) *report.StepResult {
	for i := range rpt.Phases {
		if rpt.Phases[i].Name != phase {
			continue
		}
		for j := range rpt.Phases[i].Steps {
			if rpt.Phases[i].Steps[j].Name == name {
				return &rpt.Phases[i].Steps[j]
			}
		}
	}
	return nil
}

// A dotfile gated to a host that is never the current machine must be skipped
// with "other host" — not validated and reported as a warning. Regression for
// claude_settings_local false-positiving on non-work machines.
func TestCheckDotfiles_SkipsOtherHost(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"work_only": {
				Source: "settings.work.json",
				Target: "/tmp/ralph-doctor-test-nonexistent",
				Action: "symlink",
				Hosts:  []string{"no-such-host-zzz"},
			},
		},
	}
	rpt := &report.Report{Command: "doctor"}
	checkDotfiles(rpt, cfg)

	step := findStep(rpt, "Dotfiles", "work_only")
	if step == nil {
		t.Fatal("expected a result for work_only")
	}
	if step.Status != report.StatusSkip {
		t.Fatalf("expected StatusSkip, got %v (msg=%q)", step.Status, step.Message)
	}
	if step.Message != "other host" {
		t.Fatalf("expected message %q, got %q", "other host", step.Message)
	}
}

// A dotfile with no host gate must NOT be skipped for host reasons — it should
// be validated like any other (here: warned, since the target does not exist).
func TestCheckDotfiles_AllHostsNotSkipped(t *testing.T) {
	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"shared": {
				Source: "shared.json",
				Target: "/tmp/ralph-doctor-test-nonexistent",
				Action: "symlink",
			},
		},
	}
	rpt := &report.Report{Command: "doctor"}
	checkDotfiles(rpt, cfg)

	step := findStep(rpt, "Dotfiles", "shared")
	if step == nil {
		t.Fatal("expected a result for shared")
	}
	if step.Status == report.StatusSkip {
		t.Fatalf("ungated dotfile was skipped: %q", step.Message)
	}
}
