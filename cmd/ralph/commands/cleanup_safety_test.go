package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
	"github.com/mad01/ralph/internal/state"
)

// C001 — the recipe-state manifest is persisted on EVERY successful apply, not
// only when cleanup is enabled. Without this, a recipe added and removed between
// two cleanup runs is invisible to the next cleanup and leaks forever.
func TestRecordManifest_SavesEvenWithoutCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {Source: "foo.conf", Target: filepath.Join(home, "foo.conf"), OwnerRecipe: "fooer"},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "fooer", DeleteBehavior: "delete"}},
	}

	rpt := &report.Report{Command: "up"}
	// shouldCleanup=false: cleanup is OFF, but the manifest must still be saved.
	recordManifestAndCleanup(cfg, "anyhost", false, false, &bytes.Buffer{}, rpt)

	saved, err := state.Load()
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	rec, ok := saved.Recipes["fooer"]
	if !ok {
		t.Fatalf("expected recipe 'fooer' recorded without cleanup, got %v", saved.Recipes)
	}
	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != filepath.Join(home, "foo.conf") {
		t.Errorf("expected dotfile target tracked, got %v", rec.Symlinks)
	}
}

// C001 — dry-run must never write the manifest.
func TestRecordManifest_DryRunDoesNotSave(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &config.Config{
		Dotfiles: map[string]config.Dotfile{
			"foo": {Source: "foo.conf", Target: filepath.Join(home, "foo.conf"), OwnerRecipe: "fooer"},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "fooer", DeleteBehavior: "delete"}},
	}

	rpt := &report.Report{Command: "up"}
	recordManifestAndCleanup(cfg, "anyhost", false, true /*dryRun*/, &bytes.Buffer{}, rpt)

	p, _ := state.GetStatePath()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("dry-run must not write the state file, but it exists (err=%v)", err)
	}
}

// C002 — uninstall hooks fire only when a recipe is GONE entirely. A recipe that
// merely loses one artifact (still present) must NOT have its uninstall hooks
// run, or an unrelated edit would tear down a live service.
func TestRunCleanup_PartialOrphanDoesNotRunUninstallHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Two symlinks owned by "svc"; only one is dropped this apply.
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "keep.link")
	drop := filepath.Join(dir, "drop.link")
	for _, l := range []string{keep, drop} {
		if err := os.Symlink(target, l); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(dir, "uninstall.ran")

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("svc", state.KindSymlink, keep)
	prev.AddArtifact("svc", state.KindSymlink, drop)
	prev.SetMetadata("svc", time.Now(), "delete")
	prev.SetUninstallHooks("svc", []string{"printf x > " + sentinel}, nil)

	// svc is still present, but no longer declares `drop`.
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	next.AddArtifact("svc", state.KindSymlink, keep)
	next.SetMetadata("svc", time.Now(), "delete")

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Errorf("uninstall hook must NOT run for a partial orphan, but sentinel exists")
	}
	if _, err := os.Lstat(drop); !os.IsNotExist(err) {
		t.Errorf("the dropped symlink should still be removed, lstat err=%v", err)
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Errorf("the still-declared symlink must survive, lstat err=%v", err)
	}
}

// C003 — cross-recipe protection covers ALL filesystem paths, not just
// install_paths. A symlink an active recipe still declares must not be deleted
// as an orphan of a removed recipe (the recipe-rename hazard).
func TestRunCleanup_ProtectsSymlinkStillDeclaredByAnotherRecipe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "shared.link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	// prev attributes the symlink to the OLD recipe name (now gone).
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("old-name", state.KindSymlink, link)
	prev.SetMetadata("old-name", time.Now(), "delete")

	// next declares the SAME path under the NEW recipe name.
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	next.AddArtifact("new-name", state.KindSymlink, link)
	next.SetMetadata("new-name", time.Now(), "delete")

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Lstat(link); err != nil {
		t.Errorf("symlink still declared by another recipe must survive a rename, lstat err=%v", err)
	}
	if !strings.Contains(logger.String(), "protected symlink") {
		t.Errorf("expected a 'protected symlink' log line, got: %s", logger.String())
	}
}

// C004 — `clean --recipe` must protect against the UNFILTERED manifest. We
// simulate the command's logic: protected is computed from the full manifest
// before filtering, so even when next is scoped to one recipe, a path another
// recipe still declares survives.
func TestRunCleanup_ScopedCleanHonorsUnfilteredProtection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	bin := filepath.Join(dir, "code", "bin", "tool")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Full intended manifest: the binary is declared by an active recipe.
	fullNext := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	fullNext.AddArtifact("new-tool", state.KindInstallPath, bin)
	fullNext.SetMetadata("new-tool", time.Now(), "delete")
	protected := fullNext.AllPaths() // computed BEFORE filtering — the fix

	// Scope to the old recipe only (filterRecipe equivalent): prev has it,
	// the scoped next does not.
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("old-tool", state.KindInstallPath, bin)
	prev.SetMetadata("old-tool", time.Now(), "delete")
	scopedNext := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "clean"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, scopedNext, protected, false, logger, phase)

	if _, err := os.Stat(bin); err != nil {
		t.Errorf("scoped clean must not delete a binary another recipe declares, stat err=%v", err)
	}
}

// C005 — remote/make package clone dirs are tracked, and abandoned-with-log on
// removal (never auto-deleted).
func TestBuildIntendedManifest_TracksPackageClone(t *testing.T) {
	cfg := &config.Config{
		PackagesDir: "/tmp/ralph-pkgs",
		Packages: map[string]config.Package{
			"csl": {Source: "remote", Repo: "git@github.com:mad01/csl.git", OwnerRecipe: "packages"},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{{Name: "packages", DeleteBehavior: "delete"}},
	}
	got, err := buildIntendedManifest(cfg, "anyhost", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := got.Recipes["packages"]
	want := filepath.Join("/tmp/ralph-pkgs", "csl")
	if len(rec.PackageClones) != 1 || rec.PackageClones[0] != want {
		t.Errorf("expected package clone %q tracked, got %v", want, rec.PackageClones)
	}
}

func TestRunCleanup_PackageCloneIsAbandonedNotDeleted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	clone := filepath.Join(dir, "pkg", "mixer")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("mixer", state.KindPackageClone, clone)
	prev.SetMetadata("mixer", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Stat(clone); err != nil {
		t.Errorf("package clone must NOT be auto-deleted, stat err=%v", err)
	}
	out := logger.String()
	if !strings.Contains(out, "abandoned package clone") || !strings.Contains(out, "rm -rf "+clone) {
		t.Errorf("expected abandon log with rm command for %q, got: %s", clone, out)
	}
}

// C006 — a path that fails removal (here: a read-only parent dir makes os.Remove
// fail with permission denied) is re-tracked in the returned failed set so the
// next run retries it, instead of vanishing from the manifest as permanent
// garbage.
func TestRunCleanup_FailedRemovalIsRetracked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	roDir := filepath.Join(dir, "ro")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(roDir, "stuck.link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	// Read-only parent: lstat still works, but os.Remove(link) fails.
	if err := os.Chmod(roDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("gone", state.KindSymlink, link)
	prev.SetMetadata("gone", time.Now(), "delete")
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	failed := runCleanup(prev, next, next.AllPaths(), false, &bytes.Buffer{}, phase)

	art, ok := failed["gone"]
	if !ok {
		t.Fatalf("expected failed-removal entry for 'gone', got %v", failed)
	}
	if len(art.Symlinks) != 1 || art.Symlinks[0] != link {
		t.Fatalf("expected failed symlink %q re-tracked, got %v", link, art.Symlinks)
	}

	// Caller merges failures back into next so the next run retries.
	for name, a := range failed {
		next.MergeRetry(name, a)
	}
	if got := next.Recipes["gone"].Symlinks; len(got) != 1 || got[0] != link {
		t.Errorf("expected MergeRetry to re-track %q, got %v", link, got)
	}
	// Uninstall hooks must NOT be merged (they already ran / must not re-fire).
	if len(next.Recipes["gone"].PreUninstall) != 0 {
		t.Errorf("MergeRetry must not carry uninstall hooks, got %v", next.Recipes["gone"].PreUninstall)
	}
}
