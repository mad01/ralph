package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// withHome redirects $HOME to a temp dir and returns a cleanup function.
func withHome(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ralph-state-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	prev := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	return dir, func() {
		os.Setenv("HOME", prev)
		os.RemoveAll(dir)
	}
}

func TestPath_HonorsHome(t *testing.T) {
	dir, cleanup := withHome(t)
	defer cleanup()

	p, err := GetStatePath()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, ".config", "ralph", FileName)
	if p != want {
		t.Errorf("expected %s, got %s", want, p)
	}
}

func TestLoad_MissingFile_ReturnsEmptyState(t *testing.T) {
	_, cleanup := withHome(t)
	defer cleanup()

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if len(s.Recipes) != 0 {
		t.Errorf("expected empty Recipes, got %d entries", len(s.Recipes))
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	_, cleanup := withHome(t)
	defer cleanup()

	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("brain", KindSymlink, "/home/u/.config/brain/config.yaml")
	s.AddArtifact("brain", KindDirectory, "/home/u/.config/brain/index")
	s.AddArtifact("brain", KindInstallPath, "/home/u/code/bin/brain")
	s.SetMetadata("brain", now, "delete")

	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := loaded.Recipes["brain"]
	if !got.AppliedAt.Equal(now) {
		t.Errorf("expected AppliedAt %v, got %v", now, got.AppliedAt)
	}
	if got.DeleteBehavior != "delete" {
		t.Errorf("expected delete_behavior 'delete', got %q", got.DeleteBehavior)
	}
	if !reflect.DeepEqual(got.Symlinks, []string{"/home/u/.config/brain/config.yaml"}) {
		t.Errorf("symlinks: %v", got.Symlinks)
	}
	if !reflect.DeepEqual(got.Directories, []string{"/home/u/.config/brain/index"}) {
		t.Errorf("directories: %v", got.Directories)
	}
	if !reflect.DeepEqual(got.InstallPaths, []string{"/home/u/code/bin/brain"}) {
		t.Errorf("install_paths: %v", got.InstallPaths)
	}
}

func TestSave_DedupesAndSorts(t *testing.T) {
	_, cleanup := withHome(t)
	defer cleanup()

	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("foo", KindSymlink, "/b")
	s.AddArtifact("foo", KindSymlink, "/a")
	s.AddArtifact("foo", KindSymlink, "/a") // duplicate
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, _ := Load()
	want := []string{"/a", "/b"}
	if !reflect.DeepEqual(loaded.Recipes["foo"].Symlinks, want) {
		t.Errorf("expected sorted+deduped %v, got %v", want, loaded.Recipes["foo"].Symlinks)
	}
}

func TestAddArtifact_IgnoresEmpty(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("", KindSymlink, "/x")
	s.AddArtifact("foo", KindSymlink, "")
	if len(s.Recipes) != 0 {
		t.Errorf("expected empty state, got %d entries", len(s.Recipes))
	}
}

func TestDiff_RemovedArtifact_AppearsAsOrphan(t *testing.T) {
	prev := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	prev.AddArtifact("foo", KindSymlink, "/a")
	prev.AddArtifact("foo", KindSymlink, "/b")
	prev.SetMetadata("foo", time.Now(), "delete")

	next := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	next.AddArtifact("foo", KindSymlink, "/a") // /b dropped
	next.SetMetadata("foo", time.Now(), "delete")

	orphans := Diff(prev, next)
	if !reflect.DeepEqual(orphans["foo"].Symlinks, []string{"/b"}) {
		t.Errorf("expected /b orphan, got %v", orphans["foo"].Symlinks)
	}
}

func TestDiff_RemovedRecipe_AllArtifactsOrphaned(t *testing.T) {
	prev := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	prev.AddArtifact("foo", KindSymlink, "/a")
	prev.AddArtifact("foo", KindInstallPath, "/home/u/code/bin/foo")
	prev.SetMetadata("foo", time.Now(), "delete")

	next := &RecipeState{Recipes: map[string]RecipeArtifacts{}}

	orphans := Diff(prev, next)
	got, ok := orphans["foo"]
	if !ok {
		t.Fatal("expected foo to appear in orphans")
	}
	if !reflect.DeepEqual(got.Symlinks, []string{"/a"}) {
		t.Errorf("symlinks: %v", got.Symlinks)
	}
	if !reflect.DeepEqual(got.InstallPaths, []string{"/home/u/code/bin/foo"}) {
		t.Errorf("install_paths: %v", got.InstallPaths)
	}
	if got.DeleteBehavior != "delete" {
		t.Errorf("expected DeleteBehavior carried forward, got %q", got.DeleteBehavior)
	}
}

func TestDiff_NoChanges_NoOrphans(t *testing.T) {
	prev := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	prev.AddArtifact("foo", KindSymlink, "/a")
	prev.SetMetadata("foo", time.Now(), "delete")

	next := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	next.AddArtifact("foo", KindSymlink, "/a")
	next.SetMetadata("foo", time.Now(), "delete")

	orphans := Diff(prev, next)
	if len(orphans) != 0 {
		t.Errorf("expected no orphans, got %d", len(orphans))
	}
}

func TestDiff_NilPrev_NoOrphans(t *testing.T) {
	next := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	next.AddArtifact("foo", KindSymlink, "/a")
	if len(Diff(nil, next)) != 0 {
		t.Error("expected no orphans when prev is nil")
	}
	empty := &RecipeState{}
	if len(Diff(empty, next)) != 0 {
		t.Error("expected no orphans when prev has no recipes")
	}
}

func TestHasAny_FalseForEmpty(t *testing.T) {
	if (RecipeArtifacts{}).HasAny() {
		t.Error("expected HasAny to be false for empty artifacts")
	}
	if !(RecipeArtifacts{Symlinks: []string{"/a"}}).HasAny() {
		t.Error("expected HasAny to be true with symlinks")
	}
}

func TestDeleteRecipe_RemovesEntry(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("alpha", KindSymlink, "/a")
	s.SetMetadata("alpha", time.Now(), "delete")
	s.AddArtifact("beta", KindSymlink, "/b")
	s.SetMetadata("beta", time.Now(), "delete")

	s.DeleteRecipe("alpha")

	if _, ok := s.Recipes["alpha"]; ok {
		t.Error("expected alpha to be removed")
	}
	if _, ok := s.Recipes["beta"]; !ok {
		t.Error("expected beta to remain")
	}
	if len(s.Recipes["beta"].Symlinks) != 1 || s.Recipes["beta"].Symlinks[0] != "/b" {
		t.Errorf("expected beta symlinks [/b], got %v", s.Recipes["beta"].Symlinks)
	}
}

func TestDeleteRecipe_NonExistent_NoOp(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("alpha", KindSymlink, "/a")
	s.SetMetadata("alpha", time.Now(), "delete")

	s.DeleteRecipe("nonexistent") // should not panic

	if len(s.Recipes) != 1 {
		t.Errorf("expected 1 recipe to remain, got %d", len(s.Recipes))
	}
	if _, ok := s.Recipes["alpha"]; !ok {
		t.Error("expected alpha to remain unchanged")
	}
}

func TestLoad_InvalidJSON_Errors(t *testing.T) {
	dir, cleanup := withHome(t)
	defer cleanup()
	stateDir := filepath.Join(dir, ".config", "ralph")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
