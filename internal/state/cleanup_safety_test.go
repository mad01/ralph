package state

import (
	"testing"
	"time"
)

// C002 — Diff attaches uninstall hooks ONLY when the recipe is gone entirely.
func TestDiff_UninstallHooksOnlyOnFullRemoval(t *testing.T) {
	prev := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	prev.AddArtifact("svc", KindSymlink, "/home/u/a")
	prev.AddArtifact("svc", KindSymlink, "/home/u/b")
	prev.SetMetadata("svc", time.Now(), "delete")
	prev.SetUninstallHooks("svc", []string{"t-man remove svc"}, []string{"echo bye"})

	// Partial removal: svc still present, dropped /home/u/b.
	partial := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	partial.AddArtifact("svc", KindSymlink, "/home/u/a")
	partial.SetMetadata("svc", time.Now(), "delete")

	got := Diff(prev, partial)["svc"]
	if len(got.PreUninstall) != 0 || len(got.PostUninstall) != 0 {
		t.Errorf("partial orphan must not carry uninstall hooks, got pre=%v post=%v", got.PreUninstall, got.PostUninstall)
	}

	// Full removal: svc gone entirely.
	empty := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	gone := Diff(prev, empty)["svc"]
	if len(gone.PreUninstall) != 1 || len(gone.PostUninstall) != 1 {
		t.Errorf("full removal must carry uninstall hooks, got pre=%v post=%v", gone.PreUninstall, gone.PostUninstall)
	}
}

// C003 — AllPaths flattens every filesystem-removable kind across all recipes.
func TestAllPaths_CoversAllKindsAcrossRecipes(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("a", KindSymlink, "/home/u/link")
	s.AddArtifact("a", KindCopy, "/home/u/copy")
	s.AddArtifact("b", KindDirSymlink, "/home/u/dirlink")
	s.AddArtifact("b", KindDirectory, "/home/u/dir")
	s.AddArtifact("b", KindInstallPath, "/home/u/code/bin/tool")
	// Excluded kinds: package clones and repos are never auto-removed.
	s.AddArtifact("b", KindPackageClone, "/home/u/pkg/clone")
	s.AddArtifact("b", KindRepo, "/home/u/repo")

	got := s.AllPaths()
	for _, p := range []string{"/home/u/link", "/home/u/copy", "/home/u/dirlink", "/home/u/dir", "/home/u/code/bin/tool"} {
		if !got[p] {
			t.Errorf("expected %q in AllPaths, missing", p)
		}
	}
	if got["/home/u/pkg/clone"] || got["/home/u/repo"] {
		t.Errorf("package clones and repos must be excluded from AllPaths, got %v", got)
	}
}

// C006 — MergeRetry re-tracks failed paths and preserves delete_behavior, but
// never carries uninstall hooks (they must not re-fire on retry).
func TestMergeRetry_ReTracksPathsWithoutHooks(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	art := RecipeArtifacts{
		DeleteBehavior: "delete",
		Symlinks:       []string{"/home/u/stuck"},
		InstallPaths:   []string{"/home/u/code/bin/stuck"},
		PreUninstall:   []string{"should NOT be carried"},
	}
	s.MergeRetry("gone", art)

	rec := s.Recipes["gone"]
	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != "/home/u/stuck" {
		t.Errorf("expected symlink re-tracked, got %v", rec.Symlinks)
	}
	if len(rec.InstallPaths) != 1 {
		t.Errorf("expected install_path re-tracked, got %v", rec.InstallPaths)
	}
	if rec.DeleteBehavior != "delete" {
		t.Errorf("expected delete_behavior preserved, got %q", rec.DeleteBehavior)
	}
	if len(rec.PreUninstall) != 0 {
		t.Errorf("MergeRetry must not carry uninstall hooks, got %v", rec.PreUninstall)
	}
}
