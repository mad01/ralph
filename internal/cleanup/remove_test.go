package cleanup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

func TestRunCleanup_DeleteRemovesOrphanedSymlinks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	link := filepath.Join(dir, "old.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: logger, Phase: phase})

	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected orphan symlink removed, lstat err=%v", err)
	}
	if !strings.Contains(logger.String(), "removed symlink") {
		t.Errorf("expected log line for removal, got: %s", logger.String())
	}
}

func TestRunCleanup_RunsUninstallHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	link := filepath.Join(dir, "old.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	preSentinel := filepath.Join(dir, "pre.done")
	postSentinel := filepath.Join(dir, "post.done")

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("svc", state.KindSymlink, link)
	prev.SetMetadata("svc", time.Now(), "delete")
	prev.SetUninstallHooks(
		"svc",
		[]string{"printf pre > " + preSentinel},
		[]string{"printf post > " + postSentinel},
	)
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: logger, Phase: phase})

	if _, err := os.Stat(preSentinel); err != nil {
		t.Errorf("expected pre_uninstall hook to run (sentinel missing): %v", err)
	}
	if _, err := os.Stat(postSentinel); err != nil {
		t.Errorf("expected post_uninstall hook to run (sentinel missing): %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("expected orphan symlink removed alongside hooks, lstat err=%v", err)
	}
}

func TestRunCleanup_DryRunSkipsUninstallHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sentinel := filepath.Join(dir, "ran.done")
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("svc", state.KindShellAlias, "somealias")
	prev.SetMetadata("svc", time.Now(), "delete")
	prev.SetUninstallHooks("svc", []string{"printf x > " + sentinel}, nil)
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(
		prev,
		next,
		RunOptions{Protected: next.AllPaths(), DryRun: true, Out: logger, Phase: phase},
	) // dryRun=true

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("dry run must not execute uninstall hooks, but sentinel exists (err=%v)", err)
	}
	if !strings.Contains(logger.String(), "[DRY RUN] Would run hook") {
		t.Errorf("expected dry-run hook preview in output, got: %s", logger.String())
	}
}

func TestRunCleanup_AbandonStillRunsUninstallHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sentinel := filepath.Join(dir, "ran.done")
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("svc", state.KindShellFunc, "somefunc")
	prev.SetMetadata("svc", time.Now(), "abandon")
	prev.SetUninstallHooks("svc", []string{"printf x > " + sentinel}, nil)
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: logger, Phase: phase})

	// Even though artifacts are abandoned (delete_behavior=abandon), the
	// uninstall hooks still run — they clean external state, not files.
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("expected uninstall hook to run under abandon behavior: %v", err)
	}
}

func TestRunCleanup_AbandonLeavesArtifactsInPlace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	link := filepath.Join(dir, "abandon.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "abandon")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: logger, Phase: phase})

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected abandoned symlink to remain, got %v", err)
	}
	if !strings.Contains(logger.String(), "abandoned symlink") {
		t.Errorf("expected abandon log line, got: %s", logger.String())
	}
}

func TestRunCleanup_DryRunDoesNotModify(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	link := filepath.Join(dir, "dryrun.link")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, link)
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(
		prev,
		next,
		RunOptions{Protected: next.AllPaths(), DryRun: true, Out: logger, Phase: phase},
	) // dryRun=true

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected symlink to remain in dry-run, got %v", err)
	}
	if !strings.Contains(logger.String(), "would remove") {
		t.Errorf("expected 'would remove' log in dry-run, got: %s", logger.String())
	}
}

func TestRunCleanup_RepoOrphanIsAlwaysAbandoned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	repoDir := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindRepo, repoDir)
	prev.SetMetadata("fooer", time.Now(), "delete") // even with delete, repos must abandon
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: logger, Phase: phase})

	if _, err := os.Stat(repoDir); err != nil {
		t.Errorf("expected repo to be untouched, got %v", err)
	}
	if !strings.Contains(logger.String(), "abandoned repo") {
		t.Errorf("expected abandon log for repo, got: %s", logger.String())
	}
}

func TestRunCleanup_NoOrphans_NoOp(t *testing.T) {
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("fooer", state.KindSymlink, "/tmp/keep")
	prev.SetMetadata("fooer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	next.AddArtifact("fooer", state.KindSymlink, "/tmp/keep")
	next.SetMetadata("fooer", time.Now(), "delete")

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	Run(prev, next, RunOptions{Protected: next.AllPaths(), Out: &bytes.Buffer{}, Phase: phase})

	ok, _, _, _ := phase.Counts()
	if ok < 1 {
		t.Errorf(
			"expected at least one OK entry for the no-op cleanup, got phase counts: ok=%d",
			ok,
		)
	}
}
