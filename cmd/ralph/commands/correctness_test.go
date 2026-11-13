package commands

import (
	"os/exec"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// C007 — dotfilesRepoHead returns the repo HEAD for a git checkout and "" for a
// non-git path, so the up command can detect a sync that advanced the repo.
func TestDotfilesRepoHead(t *testing.T) {
	if got := dotfilesRepoHead(&config.Config{DotfilesRepoPath: t.TempDir()}); got != "" {
		t.Errorf("expected empty HEAD for a non-git dir, got %q", got)
	}

	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	run("commit", "--allow-empty", "-m", "init")

	got := dotfilesRepoHead(&config.Config{DotfilesRepoPath: repo})
	if len(got) != 40 {
		t.Errorf("expected a 40-char HEAD sha, got %q", got)
	}
}
