package commands

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

// Fix #1 — cleanup must NOT remove an install_path that is still declared by
// an active package/build in the intended (next) manifest, even when prev
// attributed that same path to a different recipe name (recipe renamed,
// re-attributed across ralph versions, or moved between recipes). This is the
// stale-state false-orphan that deleted ~/code/bin/ralph and ~/code/bin/ks.
func TestRunCleanup_ProtectsInstallPathStillDeclaredUnderAnotherRecipe(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-ip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	_ = os.Setenv("HOME", dir)
	defer func() { _ = os.Unsetenv("HOME") }()

	bin := filepath.Join(dir, "code", "bin", "ralph")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// prev attributed the binary to an old/renamed recipe name.
	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("ralph-old", state.KindInstallPath, bin)
	prev.SetMetadata("ralph-old", time.Now(), "delete")

	// next declares the SAME path under the current recipe name.
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	next.AddArtifact("ralph", state.KindInstallPath, bin)
	next.SetMetadata("ralph", time.Now(), "delete")

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Stat(bin); err != nil {
		t.Errorf("still-declared binary must NOT be removed, stat err=%v", err)
	}
	if !strings.Contains(logger.String(), "protected install_path") {
		t.Errorf("expected a 'protected install_path' log line, got: %s", logger.String())
	}
}

// Regression guard — a genuinely orphaned install_path (declared nowhere in
// next, e.g. a disabled/removed package) must still be removed. The fix must
// not neuter legitimate cleanup.
func TestRunCleanup_RemovesTrulyOrphanedInstallPath(t *testing.T) {
	dir, err := os.MkdirTemp("", "ralph-cleanup-ip-orphan-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	_ = os.Setenv("HOME", dir)
	defer func() { _ = os.Unsetenv("HOME") }()

	bin := filepath.Join(dir, "code", "bin", "gone")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	prev := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}
	prev.AddArtifact("removed-recipe", state.KindInstallPath, bin)
	prev.SetMetadata("removed-recipe", time.Now(), "delete")

	// next declares nothing — the package/recipe is gone.
	next := &state.RecipeState{Recipes: map[string]state.RecipeArtifacts{}}

	rpt := &report.Report{Command: "test"}
	phase := rpt.AddPhase("Cleanup")
	logger := &bytes.Buffer{}
	runCleanup(prev, next, next.AllPaths(), false, logger, phase)

	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Errorf("truly orphaned install_path must be removed, stat err=%v", err)
	}
	if !strings.Contains(logger.String(), "removed install_path") {
		t.Errorf("expected a 'removed install_path' log line, got: %s", logger.String())
	}
}
