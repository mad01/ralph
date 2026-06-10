package commands

import (
	"os/exec"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// C008 — firstFailedDependency reports the first depends_on entry that failed,
// so the caller can skip a node rather than build it against a missing artifact.
func TestFirstFailedDependency(t *testing.T) {
	failed := map[string]bool{"packages.broken": true}

	if dep := firstFailedDependency([]string{"builds.ok", "packages.broken"}, failed); dep != "packages.broken" {
		t.Errorf("expected to report the failed dependency, got %q", dep)
	}
	if dep := firstFailedDependency([]string{"builds.ok", "packages.fine"}, failed); dep != "" {
		t.Errorf("expected no failed dependency, got %q", dep)
	}
	if dep := firstFailedDependency(nil, failed); dep != "" {
		t.Errorf("expected empty for no deps, got %q", dep)
	}
}

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
