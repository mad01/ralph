package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Fix #2 — SafeRemove must never delete the currently-running binary, even
// when it is a valid install_path orphan under all the other rails. The
// running executable is injected via SafeRemoveOptions.Self so the test is
// deterministic (production resolves os.Executable()).

func TestSafeRemove_RefusesToDeleteRunningExecutable(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	self := filepath.Join(dir, "ralph")
	if err := os.WriteFile(self, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts.Self = self // pretend this is the running binary

	err := SafeRemove(self, KindInstallPath, opts)
	if !errors.Is(err, ErrSelfDelete) {
		t.Fatalf("expected ErrSelfDelete, got %v", err)
	}
	if _, statErr := os.Stat(self); statErr != nil {
		t.Errorf("running executable must not be deleted, stat err=%v", statErr)
	}
}

func TestSafeRemove_RefusesSelfViaSymlink(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	real := filepath.Join(dir, "ralph.real")
	if err := os.WriteFile(real, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "ralph") // PATH entry symlinking to the real binary
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	opts.Self = real // the running binary is the real file

	// Deleting the symlink that resolves to the running binary must be refused.
	err := SafeRemove(link, KindInstallPath, opts)
	if !errors.Is(err, ErrSelfDelete) {
		t.Fatalf("expected ErrSelfDelete via symlink resolution, got %v", err)
	}
	if _, statErr := os.Lstat(link); statErr != nil {
		t.Errorf("symlink to running executable must not be deleted, lstat err=%v", statErr)
	}
}

func TestSafeRemove_DeletesNonSelfInstallPath(t *testing.T) {
	dir, opts, cleanup := withSandbox(t)
	defer cleanup()

	self := filepath.Join(dir, "ralph")
	other := filepath.Join(dir, "ks")
	for _, p := range []string{self, other} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	opts.Self = self

	if err := SafeRemove(other, KindInstallPath, opts); err != nil {
		t.Fatalf("a non-self install_path must remain removable, got %v", err)
	}
	if _, statErr := os.Stat(other); !os.IsNotExist(statErr) {
		t.Errorf("expected %s to be removed", other)
	}
}

// Fix #1 support — RecipeState.AllInstallPaths flattens install_paths across
// every recipe so cleanup can protect a still-declared binary regardless of
// which recipe name prev attributed it to.

func TestRecipeState_AllInstallPaths(t *testing.T) {
	s := &RecipeState{Recipes: map[string]RecipeArtifacts{}}
	s.AddArtifact("a", KindInstallPath, "/home/u/code/bin/x")
	s.AddArtifact("b", KindInstallPath, "/home/u/code/bin/y")
	s.AddArtifact("a", KindSymlink, "/home/u/.zshrc") // non-install_path must be ignored

	got := s.AllInstallPaths()
	if len(got) != 2 {
		t.Fatalf("expected 2 install paths, got %d (%v)", len(got), got)
	}
	if !got["/home/u/code/bin/x"] || !got["/home/u/code/bin/y"] {
		t.Errorf("missing expected install paths in %v", got)
	}
	if got["/home/u/.zshrc"] {
		t.Errorf("symlink artifact must not appear in install paths: %v", got)
	}
}

func TestRecipeState_AllInstallPaths_NilSafe(t *testing.T) {
	var s *RecipeState
	if got := s.AllInstallPaths(); len(got) != 0 {
		t.Errorf("nil state must return empty set, got %v", got)
	}
}
