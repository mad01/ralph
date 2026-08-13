package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/testutil"
)

// commitFile writes path (relative to dir) and commits it, returning the new
// commit hash.
func commitFile(t *testing.T, dir, relPath, content string) string {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGitCmd(t, dir, "add", ".")
	testutil.RunGitCmd(t, dir, "commit", "-m", "change "+relPath)
	return GetGitHash(dir)
}

func TestSubtreeChangedSince(t *testing.T) {
	dir := t.TempDir()
	first := testutil.InitGitRepo(t, dir)
	subDir := filepath.Join(dir, "sub")
	afterSub := commitFile(t, dir, "sub/tool.txt", "v1")
	afterOutside := commitFile(t, dir, "elsewhere.txt", "x")

	t.Run("changed since older commit", func(t *testing.T) {
		changed, ok := SubtreeChangedSince(dir, first)
		if !ok || !changed {
			t.Errorf("SubtreeChangedSince(root, first) = %v,%v, want true,true", changed, ok)
		}
	})

	t.Run("unchanged at HEAD", func(t *testing.T) {
		changed, ok := SubtreeChangedSince(dir, afterOutside)
		if !ok || changed {
			t.Errorf("SubtreeChangedSince(root, HEAD) = %v,%v, want false,true", changed, ok)
		}
	})

	t.Run("subtree isolation", func(t *testing.T) {
		// elsewhere.txt changed after afterSub, but sub/ did not — a check
		// scoped to sub/ must not report the unrelated change.
		changed, ok := SubtreeChangedSince(subDir, afterSub)
		if !ok || changed {
			t.Errorf("SubtreeChangedSince(sub, afterSub) = %v,%v, want false,true", changed, ok)
		}
	})

	t.Run("subtree change detected", func(t *testing.T) {
		changed, ok := SubtreeChangedSince(subDir, first)
		if !ok || !changed {
			t.Errorf("SubtreeChangedSince(sub, first) = %v,%v, want true,true", changed, ok)
		}
	})

	t.Run("unknown commit yields no verdict", func(t *testing.T) {
		if _, ok := SubtreeChangedSince(dir, "0000000000000000000000000000000000000000"); ok {
			t.Error("expected ok=false for a commit not present locally")
		}
	})

	t.Run("unsafe ref yields no verdict", func(t *testing.T) {
		if _, ok := SubtreeChangedSince(dir, "--exec=evil"); ok {
			t.Error("expected ok=false for an option-shaped ref")
		}
	})

	t.Run("non-git dir yields no verdict", func(t *testing.T) {
		if _, ok := SubtreeChangedSince(t.TempDir(), first); ok {
			t.Error("expected ok=false outside a git repository")
		}
	})
}
