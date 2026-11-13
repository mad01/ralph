package state

import (
	"slices"
	"testing"
	"time"
)

// CarryOrphans keeps what a run without cleanup did not remove: the next
// Diff against a fresh manifest reports the same orphans, hooks only for the
// recipe that is gone entirely.
func TestCarryOrphans(t *testing.T) {
	prev := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	prev.AddArtifact("gone", KindSymlink, "/h/gone.link")
	prev.AddArtifact("gone", KindPackage, "gone-pkg")
	prev.SetMetadata("gone", time.Unix(1700000000, 0), "abandon")
	prev.SetRecipeSource("gone", "moon")
	prev.SetUninstallHooks("gone", []string{"pre"}, []string{"post"})
	prev.AddArtifact("svc", KindSymlink, "/h/keep.link")
	prev.AddArtifact("svc", KindSymlink, "/h/drop.link")
	prev.SetUninstallHooks("svc", []string{"svc-pre"}, nil)

	fresh := func() *RecipeState {
		s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
		s.AddArtifact("svc", KindSymlink, "/h/keep.link")
		s.SetUninstallHooks("svc", []string{"svc-pre"}, nil)
		return s
	}
	next := fresh()
	next.CarryOrphans(Diff(prev, next))

	gone := next.Recipes["gone"]
	if !gone.PendingRemoval {
		t.Error("recipe gone entirely is not marked pending removal")
	}
	if gone.DeleteBehavior != "abandon" || gone.RecipeSource != "moon" {
		t.Errorf(
			"carried metadata = %q/%q, want abandon/moon",
			gone.DeleteBehavior,
			gone.RecipeSource,
		)
	}
	if !slices.Equal(gone.PreUninstall, []string{"pre"}) ||
		!slices.Equal(gone.PostUninstall, []string{"post"}) {
		t.Errorf("carried hooks = %v/%v, want [pre]/[post]", gone.PreUninstall, gone.PostUninstall)
	}
	svc := next.Recipes["svc"]
	if svc.PendingRemoval {
		t.Error("still-active recipe is marked pending removal")
	}
	if !slices.Contains(svc.Symlinks, "/h/drop.link") {
		t.Errorf("partial orphan not carried, symlinks = %v", svc.Symlinks)
	}

	// The following run diffs the saved state against a fresh manifest.
	again := Diff(next, fresh())
	if got := again["gone"]; !slices.Equal(got.Symlinks, []string{"/h/gone.link"}) ||
		!slices.Equal(got.Packages, []string{"gone-pkg"}) ||
		!slices.Equal(got.PreUninstall, []string{"pre"}) {
		t.Errorf("gone orphan after carry = %+v", got)
	}
	got := again["svc"]
	if !slices.Equal(got.Symlinks, []string{"/h/drop.link"}) {
		t.Errorf("svc orphan after carry = %v, want [/h/drop.link]", got.Symlinks)
	}
	if len(got.PreUninstall) != 0 {
		t.Errorf("partial orphan carries uninstall hooks %v", got.PreUninstall)
	}
}

// A failed removal of a recipe that is gone entirely re-tracks it as pending
// removal; a failure inside an active recipe does not flag that recipe.
func TestMergeRetry_MarksGoneRecipePendingRemoval(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("active", KindSymlink, "/h/a.link")

	s.MergeRetry("gone", RecipeArtifacts{Symlinks: []string{"/h/g.link"}})
	s.MergeRetry("active", RecipeArtifacts{Symlinks: []string{"/h/stuck.link"}})

	if !s.Recipes["gone"].PendingRemoval {
		t.Error("re-tracked gone recipe is not marked pending removal")
	}
	if s.Recipes["active"].PendingRemoval {
		t.Error("active recipe is marked pending removal")
	}
}
