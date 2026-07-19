package commands

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
	"github.com/mad01/ralph/internal/report"
)

// makeSourceFixture creates a local origin repo and a source checkout of it
// under a fake HOME's sources dir, returning the origin path and checkout path.
func makeSourceFixture(t *testing.T, name, ref string) (origin, checkout string) {
	t.Helper()
	origin = t.TempDir()

	gitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	gitIn(origin, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(origin, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(origin, "add", ".")
	gitIn(origin, "commit", "-m", "first")
	gitIn(origin, "tag", "v1.0.0")

	home := t.TempDir()
	t.Setenv("HOME", home)

	sourcesDir, err := config.SourcesDir()
	if err != nil {
		t.Fatal(err)
	}
	src := config.RecipeSource{Name: name, URL: origin, Ref: ref}
	checkout, err = config.EnsureSourceCheckout(io.Discard, src, sourcesDir)
	if err != nil {
		t.Fatalf("ensure failed: %v", err)
	}
	return origin, checkout
}

func advanceOrigin(t *testing.T, origin string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(origin, "b.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "second"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return gitutil.GetGitHash(origin)
}

func TestSyncRecipeSources_PullsBranchSource(t *testing.T) {
	origin, checkout := makeSourceFixture(t, "moon", "main")
	second := advanceOrigin(t, origin)

	cfg := &config.Config{
		RecipeSources: []config.RecipeSource{
			{Name: "moon", URL: origin, Ref: "main", Update: true},
		},
	}
	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Recipe sources")

	if ok := syncRecipeSources(io.Discard, cfg, phase, false); !ok {
		t.Fatal("sync reported failure")
	}
	if got := gitutil.GetGitHash(checkout); got != second {
		t.Errorf("HEAD = %s, want %s after pull", got, second)
	}
}

func TestSyncRecipeSources_SkipsPinnedAndNoUpdate(t *testing.T) {
	origin, checkout := makeSourceFixture(t, "moon", "v1.0.0")
	before := gitutil.GetGitHash(checkout)
	advanceOrigin(t, origin)

	cfg := &config.Config{
		RecipeSources: []config.RecipeSource{
			// Pinned to a tag: detached HEAD, update must be a no-op.
			{Name: "moon", URL: origin, Ref: "v1.0.0", Update: true},
		},
	}
	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Recipe sources")

	if ok := syncRecipeSources(io.Discard, cfg, phase, false); !ok {
		t.Fatal("sync reported failure")
	}
	if got := gitutil.GetGitHash(checkout); got != before {
		t.Errorf("pinned checkout moved: %s -> %s", before, got)
	}
}

func TestSyncFingerprint_ChangesWhenSourceAdvances(t *testing.T) {
	origin, checkout := makeSourceFixture(t, "moon", "main")

	cfg := &config.Config{
		DotfilesRepoPath: t.TempDir(),
		RecipeSources: []config.RecipeSource{
			{Name: "moon", URL: origin, Ref: "main", Update: true},
		},
	}

	before := syncFingerprint(cfg)
	advanceOrigin(t, origin)
	if err := gitutil.Pull(io.Discard, checkout); err != nil {
		t.Fatalf("pull failed: %v", err)
	}
	after := syncFingerprint(cfg)
	if before == after {
		t.Error("fingerprint unchanged after source advanced")
	}
}
