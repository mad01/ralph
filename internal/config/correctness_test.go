package config

import (
	"strings"
	"testing"
)

// C011 — a depends_on whose target runs in a LATER wave is flagged; same-wave
// and earlier-wave dependencies are not.
func TestCrossWaveDependencyWarnings(t *testing.T) {
	builds := map[string]Build{
		"early":    {Wave: 0},
		"late":     {Wave: 2, DependsOn: []string{"builds.early"}},      // earlier dep — fine
		"badorder": {Wave: 0, DependsOn: []string{"packages.laterpkg"}}, // later dep — WARN
	}
	pkgs := map[string]Package{
		"laterpkg": {Wave: 1},
		"samewave": {Wave: 0, DependsOn: []string{"builds.early"}}, // same wave — fine
	}

	got := CrossWaveDependencyWarnings(builds, pkgs)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 cross-wave warning, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "builds.badorder") || !strings.Contains(got[0], "packages.laterpkg") {
		t.Errorf("warning should name the offending edge, got: %s", got[0])
	}
}

func TestCrossWaveDependencyWarnings_NoneWhenWellOrdered(t *testing.T) {
	builds := map[string]Build{
		"base": {Wave: 0},
		"app":  {Wave: 1, DependsOn: []string{"builds.base"}},
	}
	if got := CrossWaveDependencyWarnings(builds, nil); len(got) != 0 {
		t.Errorf("expected no warnings for well-ordered waves, got: %v", got)
	}
}

// C015 — is_template requires action = "copy"; symlink/symlink_dir/default are
// rejected at validation.
func TestValidateDotfiles_TemplateRequiresCopy(t *testing.T) {
	mk := func(action string) map[string]Dotfile {
		return map[string]Dotfile{
			"cfg": {Source: "cfg.tmpl", Target: "/tmp/cfg", IsTemplate: true, Action: action},
		}
	}

	if err := validateDotfiles(mk("copy")); err != nil {
		t.Errorf("is_template + copy must be valid, got: %v", err)
	}
	for _, bad := range []string{"", "symlink", "symlink_dir"} {
		if err := validateDotfiles(mk(bad)); err == nil {
			t.Errorf("is_template + action=%q must be rejected, got nil error", bad)
		}
	}

	// A non-template dotfile is unaffected.
	ok := map[string]Dotfile{"plain": {Source: "x", Target: "/tmp/x"}}
	if err := validateDotfiles(ok); err != nil {
		t.Errorf("non-template dotfile must remain valid, got: %v", err)
	}
}
