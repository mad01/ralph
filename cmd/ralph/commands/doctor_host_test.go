package commands

import (
	"os"
	"path/filepath"
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

// A dotfile gated to a profile the machine does not have must be skipped with
// "other profile" — not validated and reported as a warning.
func TestCheckDotfiles_SkipsOtherProfile(t *testing.T) {
	cfg := &config.Config{
		Profiles: []string{"personal"},
		Dotfiles: map[string]config.Dotfile{
			"work_only": {
				Source:   "settings.work.json",
				Target:   "/tmp/ralph-doctor-test-nonexistent",
				Action:   "symlink",
				Profiles: []string{"work"},
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
	if step.Message != "other profile" {
		t.Fatalf("expected message %q, got %q", "other profile", step.Message)
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

// withDoctorConfig writes a config.toml (and optional config.local.toml) into a
// temp dir and points GetDefaultConfigPath at it for the duration of the test.
func withDoctorConfig(t *testing.T, localBody string) {
	t.Helper()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(mainPath, []byte(`dotfiles_repo_path = "~/dots"`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if localBody != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(localBody), 0o644); err != nil {
			t.Fatalf("write local config: %v", err)
		}
	}
	orig := config.GetDefaultConfigPath
	config.GetDefaultConfigPath = func() (string, error) { return mainPath, nil }
	t.Cleanup(func() { config.GetDefaultConfigPath = orig })
}

// A missing config.local.toml overlay must surface as a warning (the machine
// has no profiles), not a silent skip.
func TestCheckConfig_MissingOverlayWarns(t *testing.T) {
	withDoctorConfig(t, "")

	rpt := &report.Report{Command: "doctor"}
	checkConfig(rpt, &config.Config{})

	overlay := findStep(rpt, "Configuration", "config.local.toml")
	if overlay == nil {
		t.Fatal("expected a config.local.toml result")
	}
	if overlay.Status != report.StatusWarn {
		t.Fatalf("missing overlay status = %v, want warn (msg=%q)", overlay.Status, overlay.Message)
	}
}

// An overlay that exists but sets no profiles is still a warning.
func TestCheckConfig_OverlayWithoutProfilesWarns(t *testing.T) {
	withDoctorConfig(t, `packages_dir = "/tmp/pkg"`)

	rpt := &report.Report{Command: "doctor"}
	checkConfig(rpt, &config.Config{}) // overlay present on disk, but no profiles parsed

	overlay := findStep(rpt, "Configuration", "config.local.toml")
	if overlay == nil || overlay.Status != report.StatusWarn {
		t.Fatalf("overlay-without-profiles = %v, want warn", overlay)
	}
}

// With the overlay present and machine profiles set, the result is OK and lists
// them.
func TestCheckConfig_ReportsProfiles(t *testing.T) {
	withDoctorConfig(t, `profiles = ["personal", "homelab"]`)

	rpt := &report.Report{Command: "doctor"}
	checkConfig(rpt, &config.Config{Profiles: []string{"personal", "homelab"}})

	overlay := findStep(rpt, "Configuration", "config.local.toml")
	if overlay == nil {
		t.Fatal("expected a config.local.toml result")
	}
	if overlay.Status != report.StatusOK {
		t.Fatalf("overlay status = %v, want OK", overlay.Status)
	}
	if overlay.Message != "loaded: personal, homelab" {
		t.Fatalf("overlay message = %q, want %q", overlay.Message, "loaded: personal, homelab")
	}
}
