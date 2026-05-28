package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/testutil"
)

func TestGetGitHash_ReturnsHash(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	hash := testutil.InitGitRepo(t, repoDir)

	got := GetGitHash(repoDir)
	if got != hash {
		t.Errorf("GetGitHash() = %q, want %q", got, hash)
	}
	if len(got) != 40 {
		t.Errorf("expected 40-char hash, got %d chars", len(got))
	}
}

func TestGetGitHash_NonGitDir_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := GetGitHash(dir)
	if got != "" {
		t.Errorf("GetGitHash(non-git) = %q, want empty", got)
	}
}

func TestHasGitChanges_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	if HasGitChanges(repoDir) {
		t.Error("expected no changes in clean repo")
	}
}

func TestHasGitChanges_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	testutil.InitGitRepo(t, repoDir)

	os.WriteFile(filepath.Join(repoDir, "test.txt"), []byte("modified"), 0644)

	if !HasGitChanges(repoDir) {
		t.Error("expected changes after modifying tracked file")
	}
}

func TestHasGitChanges_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	if HasGitChanges(dir) {
		t.Error("expected false for non-git directory")
	}
}
