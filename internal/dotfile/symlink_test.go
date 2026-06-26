package dotfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// Helper to create a dummy file and its parent dirs
func createDummyFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("Failed to create parent dirs for dummy file %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write dummy file %s: %v", path, err)
	}
}

func TestCreateSymlink_DryRun_Simple(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	_ = os.MkdirAll(dotfilesRepo, 0o755)

	df := config.Dotfile{Source: "source.txt", Target: filepath.Join(tempDir, "target.txt")}
	absoluteSourcePath := filepath.Join(dotfilesRepo, df.Source)
	createDummyFile(t, absoluteSourcePath, "source content")

	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, true)
	if err != nil {
		t.Errorf("CreateSymlink dry run returned error: %v", err)
	}

	// Check that target symlink was NOT created
	_, statErr := os.Lstat(df.Target)
	if !os.IsNotExist(statErr) {
		t.Errorf(
			"CreateSymlink dry run created a file/symlink at target %s when it should not have",
			df.Target,
		)
	}
}

func TestCreateSymlink_ActualCreate_NoTargetConflict(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	_ = os.MkdirAll(dotfilesRepo, 0o755)

	dfSourceFilename := "actual_source.txt"
	dfTargetFilename := "actual_target.txt"

	df := config.Dotfile{
		Source: dfSourceFilename,
		Target: filepath.Join(tempDir, "link_dir", dfTargetFilename), // Target in a subdir
	}
	absoluteSourcePath := filepath.Join(dotfilesRepo, df.Source)
	createDummyFile(t, absoluteSourcePath, "hello world")

	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false)
	if err != nil {
		t.Fatalf("CreateSymlink failed: %v", err)
	}

	// Verify symlink
	targetPath, _ := config.ExpandPath(df.Target)
	linkDest, readErr := os.Readlink(targetPath)
	if readErr != nil {
		t.Fatalf("Could not read link at %s: %v", targetPath, readErr)
	}
	expandedSource, _ := config.ExpandPath(absoluteSourcePath)
	if linkDest != expandedSource {
		t.Errorf("Symlink at %s points to %s, expected %s", targetPath, linkDest, expandedSource)
	}
}

func TestCreateSymlink_SourceDoesNotExist(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	_ = os.MkdirAll(dotfilesRepo, 0o755)

	df := config.Dotfile{
		Source: "non_existent_source.txt",
		Target: filepath.Join(tempDir, "target.txt"),
	}

	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false)
	if err == nil {
		t.Errorf("CreateSymlink did not return an error when source does not exist")
	} else {
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("CreateSymlink error message '%s' did not contain expected phrase 'does not exist'", err.Error())
		}
	}
}

func TestCreateSymlink_TargetExists_SkipAction(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")

	// Case 1: Target is already correct symlink
	t.Run("TargetIsCorrectSymlink", func(t *testing.T) {
		createDummyFile(t, targetFilePath+".tmp_source_for_link", "original link content")
		_ = os.Symlink(targetFilePath+".tmp_source_for_link", targetFilePath)
		defer func() { _ = os.Remove(targetFilePath) }()
		defer func() { _ = os.Remove(targetFilePath + ".tmp_source_for_link") }()
		// Re-point the correct symlink to the actual source file of this test
		_ = os.Remove(targetFilePath) // remove temp symlink
		if err := os.Symlink(filepath.Join(dotfilesRepo, "source.txt"), targetFilePath); err != nil {
			t.Fatalf("Failed to set up correct symlink for test: %v", err)
		}

		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if err != nil {
			t.Errorf("SkipAction with correct symlink returned error: %v", err)
		}
	})

	// Case 2: Target is a regular file — returns ErrSkipped
	t.Run("TargetIsFile", func(t *testing.T) {
		createDummyFile(t, targetFilePath, "existing file content")
		defer func() { _ = os.Remove(targetFilePath) }()
		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if !errors.Is(err, ErrSkipped) {
			t.Errorf("SkipAction with existing file: want ErrSkipped, got %v", err)
		}
		// Check it's still the original file
		content, _ := os.ReadFile(targetFilePath)
		if string(content) != "existing file content" {
			t.Errorf("SkipAction modified the existing file content")
		}
	})

	// Case 3: Target is an incorrect symlink — returns ErrSkipped
	t.Run("TargetIsIncorrectSymlink", func(t *testing.T) {
		createDummyFile(t, filepath.Join(tempDir, "wrong_source.txt"), "wrong source")
		_ = os.Symlink(filepath.Join(tempDir, "wrong_source.txt"), targetFilePath)
		defer func() { _ = os.Remove(targetFilePath) }()
		defer func() { _ = os.Remove(filepath.Join(tempDir, "wrong_source.txt")) }()

		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if !errors.Is(err, ErrSkipped) {
			t.Errorf("SkipAction with incorrect symlink: want ErrSkipped, got %v", err)
		}
		linkDest, _ := os.Readlink(targetFilePath)
		if linkDest != filepath.Join(tempDir, "wrong_source.txt") {
			t.Errorf("SkipAction modified the incorrect symlink")
		}
	})
}

func TestCreateSymlink_TargetExists_BackupAction(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	absoluteSourcePath := filepath.Join(dotfilesRepo, "source.txt")
	createDummyFile(t, absoluteSourcePath, "new source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")
	createDummyFile(t, targetFilePath, "original target content")

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false)
	if err != nil {
		t.Fatalf("BackupAction failed: %v", err)
	}

	// Find the backup file using glob (timestamped suffix)
	matches, err := filepath.Glob(targetFilePath + ".bak.*")
	if err != nil {
		t.Fatalf("Glob for backup files failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("No backup file found matching .bak.* pattern")
	}

	// Check backup content (use the most recent match)
	sort.Strings(matches)
	backupContent, err := os.ReadFile(matches[len(matches)-1])
	if err != nil {
		t.Fatalf("Could not read backup file: %v", err)
	}
	if string(backupContent) != "original target content" {
		t.Errorf(
			"Backup content mismatch. Got '%s', want '%s'",
			string(backupContent),
			"original target content",
		)
	}

	// Check new symlink
	linkDest, readErr := os.Readlink(targetFilePath)
	if readErr != nil {
		t.Fatalf("Could not read link at %s: %v", targetFilePath, readErr)
	}
	expandedSource, _ := config.ExpandPath(absoluteSourcePath)
	if linkDest != expandedSource {
		t.Errorf(
			"Symlink at %s points to %s, expected %s",
			targetFilePath,
			linkDest,
			expandedSource,
		)
	}
}

func TestCreateSymlink_BackupSkipsWhenAlreadyCorrect(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	absoluteSourcePath := filepath.Join(dotfilesRepo, "source.txt")
	createDummyFile(t, absoluteSourcePath, "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")

	// First apply: no target yet, creates symlink
	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	// Verify symlink was created
	linkDest, err := os.Readlink(targetFilePath)
	if err != nil {
		t.Fatalf("Could not read link: %v", err)
	}
	if linkDest != absoluteSourcePath {
		t.Fatalf("Symlink points to %s, expected %s", linkDest, absoluteSourcePath)
	}

	// Second apply: symlink already correct — should NOT create a .bak file
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	matches, _ := filepath.Glob(targetFilePath + ".bak.*")
	if len(matches) != 0 {
		t.Errorf(
			"Expected no backup files when symlink already correct, got %d: %v",
			len(matches),
			matches,
		)
	}

	// Symlink should still point to the correct source
	linkDest, err = os.Readlink(targetFilePath)
	if err != nil {
		t.Fatalf("Could not read link after second apply: %v", err)
	}
	if linkDest != absoluteSourcePath {
		t.Errorf("Symlink changed to %s, expected %s", linkDest, absoluteSourcePath)
	}
}

func TestCreateDirSymlink_SkipAction_ReturnsErrSkipped(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(dotfilesRepo, "mydir")
	_ = os.MkdirAll(sourceDir, 0o755)

	targetPath := filepath.Join(tempDir, "link-to-mydir")

	df := config.Dotfile{Source: "mydir", Target: targetPath}

	t.Run("TargetIsWrongSymlink", func(t *testing.T) {
		wrongDir := filepath.Join(tempDir, "wrong-dir")
		_ = os.MkdirAll(wrongDir, 0o755)
		_ = os.Symlink(wrongDir, targetPath)
		defer func() { _ = os.Remove(targetPath) }()

		err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if !errors.Is(err, ErrSkipped) {
			t.Errorf("CreateDirSymlink SkipAction with wrong symlink: want ErrSkipped, got %v", err)
		}
	})

	t.Run("TargetIsDirectory", func(t *testing.T) {
		_ = os.MkdirAll(targetPath, 0o755)
		defer func() { _ = os.RemoveAll(targetPath) }()

		err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if !errors.Is(err, ErrSkipped) {
			t.Errorf("CreateDirSymlink SkipAction with existing dir: want ErrSkipped, got %v", err)
		}
	})

	t.Run("TargetIsCorrectSymlink_ReturnsNil", func(t *testing.T) {
		_ = os.Symlink(sourceDir, targetPath)
		defer func() { _ = os.Remove(targetPath) }()

		err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if err != nil {
			t.Errorf("CreateDirSymlink SkipAction with correct symlink: want nil, got %v", err)
		}
	})
}

func TestCreateDirSymlink_OverwriteBacksUpNonEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(dotfilesRepo, "mydir")
	_ = os.MkdirAll(sourceDir, 0o755)

	// Pre-existing NON-EMPTY directory at the target with user data inside.
	targetPath := filepath.Join(tempDir, "link-to-mydir")
	_ = os.MkdirAll(targetPath, 0o755)
	createDummyFile(t, filepath.Join(targetPath, "user-data.txt"), "precious")

	df := config.Dotfile{Source: "mydir", Target: targetPath}

	// Overwrite must NOT destroy a non-empty directory; it backs it up instead.
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionOverwrite, false); err != nil {
		t.Fatalf("CreateDirSymlink overwrite failed: %v", err)
	}

	// The target is now the symlink to the source.
	if dest, err := os.Readlink(targetPath); err != nil || dest != sourceDir {
		t.Fatalf("target should be a symlink to source, dest=%q err=%v", dest, err)
	}

	// The original data must survive in a timestamped backup directory.
	baks, _ := filepath.Glob(targetPath + ".bak.*")
	if len(baks) != 1 {
		t.Fatalf("expected one backup of the non-empty dir, got %v", baks)
	}
	data, err := os.ReadFile(filepath.Join(baks[0], "user-data.txt"))
	if err != nil || string(data) != "precious" {
		t.Errorf("user data not preserved in backup: data=%q err=%v", string(data), err)
	}
}

func TestCreateDirSymlink_OverwriteRemovesEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(dotfilesRepo, "mydir")
	_ = os.MkdirAll(sourceDir, 0o755)

	targetPath := filepath.Join(tempDir, "link-to-mydir")
	_ = os.MkdirAll(targetPath, 0o755) // empty dir

	df := config.Dotfile{Source: "mydir", Target: targetPath}
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionOverwrite, false); err != nil {
		t.Fatalf("CreateDirSymlink overwrite failed: %v", err)
	}
	if dest, err := os.Readlink(targetPath); err != nil || dest != sourceDir {
		t.Fatalf("target should be a symlink to source, dest=%q err=%v", dest, err)
	}
	// An empty dir is removed, not backed up.
	if baks, _ := filepath.Glob(targetPath + ".bak.*"); len(baks) != 0 {
		t.Errorf("empty dir should not be backed up, got %v", baks)
	}
}

func TestCreateDirSymlink_BackupSkipsWhenAlreadyCorrect(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(dotfilesRepo, "mydir")
	_ = os.MkdirAll(sourceDir, 0o755)

	targetPath := filepath.Join(tempDir, "link-to-mydir")

	df := config.Dotfile{Source: "mydir", Target: targetPath}

	// First apply: creates the dir symlink
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	linkDest, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Could not read link: %v", err)
	}
	if linkDest != sourceDir {
		t.Fatalf("Symlink points to %s, expected %s", linkDest, sourceDir)
	}

	// Second apply: already correct — should NOT create a .bak
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	matches, _ := filepath.Glob(targetPath + ".bak.*")
	if len(matches) != 0 {
		t.Errorf(
			"Expected no backup files when dir symlink already correct, got %d: %v",
			len(matches),
			matches,
		)
	}
}

func TestCleanupStaleBackups_RemovesOnlyTimestampedSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "repo", "source.txt")
	createDummyFile(t, src, "source content")

	target := filepath.Join(tempDir, "target.txt")
	_ = os.Symlink(src, target)

	// Only symlinks matching ralph's exact .bak.<timestamp> format are removed.
	staleSymlinks := []string{
		target + ".bak.20260523T230641.342329000",
		target + ".bak.20260527T215047.773867000",
	}
	for _, p := range staleSymlinks {
		if err := os.Symlink(src, p); err != nil {
			t.Fatalf("Failed to create stale symlink %s: %v", p, err)
		}
	}

	// Symlinks that share the .bak prefix but are NOT ralph's format must be
	// left alone — they may be foreign or attacker-planted (e.g. a symlink
	// named config.bak.evil sitting next to ~/.ssh/config).
	keptSymlinks := []string{
		target + ".bak",                  // bare, never ralph's symlink format
		target + ".bak.evil",             // non-timestamp suffix
		target + ".bak.20260523",         // truncated timestamp
		target + ".backup.20260523T2306", // different prefix
	}
	for _, p := range keptSymlinks {
		if err := os.Symlink(src, p); err != nil {
			t.Fatalf("Failed to create kept symlink %s: %v", p, err)
		}
	}

	// Regular-file .bak must survive (could be real user data ralph replaced)
	realBak := target + ".bak.20250101T000000.000000000"
	createDummyFile(t, realBak, "real user data")

	removed := cleanupStaleBackups(io.Discard, target, false)
	if removed != len(staleSymlinks) {
		t.Errorf("Expected %d stale backups removed, got %d", len(staleSymlinks), removed)
	}

	for _, p := range staleSymlinks {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("Stale symlink %s still exists", filepath.Base(p))
		}
	}
	for _, p := range keptSymlinks {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("Symlink %s should have been kept, got err %v", filepath.Base(p), err)
		}
	}
	if _, err := os.Stat(realBak); os.IsNotExist(err) {
		t.Error("Regular-file .bak was incorrectly removed")
	}
}

func TestCleanupStaleBackups_FailedRemovalNotCounted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions don't prevent removal")
	}
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "repo", "source.txt")
	createDummyFile(t, src, "source content")

	dir := filepath.Join(tempDir, "locked")
	_ = os.MkdirAll(dir, 0o755)
	target := filepath.Join(dir, "target.txt")
	_ = os.Symlink(src, target)
	bak := target + ".bak.20260523T230641.342329000"
	_ = os.Symlink(src, bak)

	// Make the directory read+execute only so os.Remove fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	removed := cleanupStaleBackups(io.Discard, target, false)
	if removed != 0 {
		t.Errorf("a failed removal must not be counted as removed, got %d", removed)
	}
}

func TestCleanupStaleBackups_DryRunDoesNotRemove(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "repo", "source.txt")
	createDummyFile(t, src, "source content")

	target := filepath.Join(tempDir, "target.txt")
	_ = os.Symlink(src, target)

	bak := target + ".bak.20260523T230641.342329000"
	_ = os.Symlink(src, bak)

	removed := cleanupStaleBackups(io.Discard, target, true)
	if removed != 1 {
		t.Errorf("Expected 1 stale backup reported, got %d", removed)
	}
	if _, err := os.Lstat(bak); os.IsNotExist(err) {
		t.Error("Dry run removed the .bak file")
	}
}

func TestCleanupStaleBackups_ScopedToTargetDir(t *testing.T) {
	tempDir := t.TempDir()
	src := filepath.Join(tempDir, "repo", "source.txt")
	createDummyFile(t, src, "source content")

	// Two targets in different directories
	dirA := filepath.Join(tempDir, "dir-a")
	dirB := filepath.Join(tempDir, "dir-b")
	_ = os.MkdirAll(dirA, 0o755)
	_ = os.MkdirAll(dirB, 0o755)

	targetA := filepath.Join(dirA, "config.yaml")
	targetB := filepath.Join(dirB, "config.yaml")
	_ = os.Symlink(src, targetA)
	_ = os.Symlink(src, targetB)

	// Plant stale backups in both dirs
	bakA := targetA + ".bak.20260527T181453.135585000"
	bakB := targetB + ".bak.20260527T181453.135585000"
	_ = os.Symlink(src, bakA)
	_ = os.Symlink(src, bakB)

	// Cleanup only targetA — dirB must be untouched
	cleanupStaleBackups(io.Discard, targetA, false)

	if _, err := os.Lstat(bakA); !os.IsNotExist(err) {
		t.Error("bakA should have been removed")
	}
	if _, err := os.Lstat(bakB); os.IsNotExist(err) {
		t.Error("bakB in a different directory was incorrectly removed")
	}
}

func TestCreateSymlink_CleansStaleBackupsOnAlreadyLinked(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	absoluteSourcePath := filepath.Join(dotfilesRepo, "source.txt")
	createDummyFile(t, absoluteSourcePath, "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")
	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}

	// First apply: creates symlink
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	// Plant stale ralph-format .bak symlinks plus a bare ".bak" that ralph's
	// timestamped format never produces.
	timestamped := []string{
		".bak.20260525T191256.068349000",
		".bak.20260526T165602.924932000",
		".bak.20260527T194659.064097000",
		".bak.20260527T215047.773867000",
	}
	for _, ts := range timestamped {
		_ = os.Symlink(absoluteSourcePath, targetFilePath+ts)
	}
	_ = os.Symlink(absoluteSourcePath, targetFilePath+".bak")

	// Second apply: should hit "already linked" and clean up only the
	// timestamped backups, leaving the bare ".bak" untouched.
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	matches, _ := filepath.Glob(targetFilePath + ".bak.2*")
	if len(matches) != 0 {
		t.Errorf(
			"Expected timestamped backups cleaned, got %d remaining: %v",
			len(matches),
			matches,
		)
	}
	if _, err := os.Lstat(targetFilePath + ".bak"); err != nil {
		t.Errorf("bare .bak (not ralph's format) should be left alone, got %v", err)
	}
}

func TestCreateDirSymlink_CleansStaleBackupsOnAlreadyLinked(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(dotfilesRepo, "mydir")
	_ = os.MkdirAll(sourceDir, 0o755)

	target := filepath.Join(tempDir, "link-to-mydir")
	df := config.Dotfile{Source: "mydir", Target: target}

	// First apply
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	// Plant a bare .bak (kept) and a ralph-format timestamped .bak (removed)
	_ = os.Symlink(sourceDir, target+".bak")
	_ = os.Symlink(sourceDir, target+".bak.20260527T215047.773867000")

	// Second apply: cleanup should fire
	if err := CreateDirSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	matches, _ := filepath.Glob(target + ".bak.2*")
	if len(matches) != 0 {
		t.Errorf(
			"Expected timestamped backups cleaned, got %d remaining: %v",
			len(matches),
			matches,
		)
	}
	if _, err := os.Lstat(target + ".bak"); err != nil {
		t.Errorf("bare .bak (not ralph's format) should be left alone, got %v", err)
	}
}

func TestCreateSymlink_BackupDoesNotOverwritePrevious(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")

	// First apply: create original file, back it up
	createDummyFile(t, targetFilePath, "original content")
	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	// Second apply: create another file at target, back it up
	_ = os.Remove(targetFilePath) // remove symlink from first apply
	createDummyFile(t, targetFilePath, "second content")
	if err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("Second backup failed: %v", err)
	}

	// Both backups should exist
	matches, err := filepath.Glob(targetFilePath + ".bak.*")
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(matches) < 2 {
		t.Errorf("Expected at least 2 backup files, got %d", len(matches))
	}
}

func TestCreateSymlink_TargetExists_OverwriteAction(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	absoluteSourcePath := filepath.Join(dotfilesRepo, "overwrite_source.txt")
	createDummyFile(t, absoluteSourcePath, "new source for overwrite")

	targetFilePath := filepath.Join(tempDir, "overwrite_target.txt")
	createDummyFile(t, targetFilePath, "original content to be overwritten")
	// No .bak file expected here

	df := config.Dotfile{Source: "overwrite_source.txt", Target: targetFilePath}
	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionOverwrite, false)
	if err != nil {
		t.Fatalf("OverwriteAction failed: %v", err)
	}

	// Check new symlink (target should now be a symlink)
	linkDest, readErr := os.Readlink(targetFilePath)
	if readErr != nil {
		t.Fatalf("Could not read link at %s after overwrite: %v", targetFilePath, readErr)
	}
	expandedSource, _ := config.ExpandPath(absoluteSourcePath)
	if linkDest != expandedSource {
		t.Errorf(
			"Symlink at %s points to %s, expected %s after overwrite",
			targetFilePath,
			linkDest,
			expandedSource,
		)
	}

	// Ensure original file (now a symlink) content reflects source if read through link
	linkedContent, err := os.ReadFile(targetFilePath)
	if err != nil {
		t.Fatalf("Could not read content through created symlink %s: %v", targetFilePath, err)
	}
	if string(linkedContent) != "new source for overwrite" {
		t.Errorf(
			"Content read through symlink was '%s', expected '%s'",
			string(linkedContent),
			"new source for overwrite",
		)
	}
}

// Test case for when source is already an absolute path (processed template)
func TestCreateSymlink_AbsoluteSourcePath(t *testing.T) {
	tempDir := t.TempDir()
	absoluteSourceFilePath := filepath.Join(tempDir, "processed-template-source.txt")
	createDummyFile(t, absoluteSourceFilePath, "processed template content")

	df := config.Dotfile{
		Source: absoluteSourceFilePath, // This is key: source is absolute
		Target: filepath.Join(tempDir, "target_for_abs_source.txt"),
	}

	// dotfilesRepoPath should be empty to indicate absolute source
	err := CreateSymlink(io.Discard, df, "", SymlinkActionBackup, false)
	if err != nil {
		t.Fatalf("CreateSymlink with absolute source failed: %v", err)
	}

	targetPath, _ := config.ExpandPath(df.Target)
	linkDest, readErr := os.Readlink(targetPath)
	if readErr != nil {
		t.Fatalf("Could not read link at %s: %v", targetPath, readErr)
	}
	// For absolute source, linkDest should be exactly df.Source
	if linkDest != df.Source { // df.Source is already absolute and expanded
		t.Errorf(
			"Symlink for absolute source at %s points to %s, expected %s",
			targetPath,
			linkDest,
			df.Source,
		)
	}
}
