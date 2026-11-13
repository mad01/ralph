package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

// withPath sets PATH to a temp dir, holding a fake git executable when
// withGit. It returns the fake git's path.
func withPath(t *testing.T, withGit bool) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	gitPath := filepath.Join(dir, "git")
	if withGit {
		if err := os.WriteFile(gitPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write fake git: %v", err)
		}
	}
	return gitPath
}

func TestCheckGit_MissingFails(t *testing.T) {
	withPath(t, false)
	cfg := &config.Config{DotfilesRepoPath: "~/dots"}

	rpt := &report.Report{Command: "doctor"}
	checkGit(rpt, cfg)

	step := findStep(rpt, "Prerequisites", "git")
	if step == nil || step.Status != report.StatusFail {
		t.Fatalf("git = %+v, want fail", step)
	}
	if want := "not found on PATH; needed for the dotfiles repo"; step.Message != want {
		t.Errorf("message = %q, want %q", step.Message, want)
	}
}

func TestCheckGit_PresentOK(t *testing.T) {
	gitPath := withPath(t, true)
	cfg := &config.Config{
		Repos: map[string]config.Repo{"notes": {Target: "~/notes"}},
	}

	rpt := &report.Report{Command: "doctor"}
	checkGit(rpt, cfg)

	step := findStep(rpt, "Prerequisites", "git")
	if step == nil || step.Status != report.StatusOK || step.Message != gitPath {
		t.Fatalf("git = %+v, want OK with message %q", step, gitPath)
	}
}

// Nothing in this config runs git: local packages, and a disabled repo, a
// remote package for another profile and a recipe source for another profile.
func TestCheckGit_NotNeededAddsNoPhase(t *testing.T) {
	withPath(t, false)
	off := false
	cfg := &config.Config{
		Profiles: []string{"personal"},
		Repos:    map[string]config.Repo{"notes": {Target: "~/notes", Enable: &off}},
		Packages: map[string]config.Package{
			"tool":   {Source: "local"},
			"remote": {Source: "remote", Profiles: []string{"work"}},
		},
		RecipeSources: []config.RecipeSource{{Name: "work", Profiles: []string{"work"}}},
	}

	rpt := &report.Report{Command: "doctor"}
	checkGit(rpt, cfg)

	if len(rpt.Phases) != 0 {
		t.Fatalf("a config without git users should add no phase, got %+v", rpt.Phases)
	}
}
