package dotfile

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

// Helper to create a dummy file and its parent dirs
func createDummyFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create parent dirs for dummy file %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write dummy file %s: %v", path, err)
	}
}

func TestCreateSymlink_DryRun_Simple(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	os.MkdirAll(dotfilesRepo, 0755)

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
		t.Errorf("CreateSymlink dry run created a file/symlink at target %s when it should not have", df.Target)
	}
}

func TestCreateSymlink_ActualCreate_NoTargetConflict(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	os.MkdirAll(dotfilesRepo, 0755)

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
	os.MkdirAll(dotfilesRepo, 0755)

	df := config.Dotfile{Source: "non_existent_source.txt", Target: filepath.Join(tempDir, "target.txt")}

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
		os.Symlink(targetFilePath+".tmp_source_for_link", targetFilePath)
		defer os.Remove(targetFilePath)
		defer os.Remove(targetFilePath + ".tmp_source_for_link")
		// Re-point the correct symlink to the actual source file of this test
		os.Remove(targetFilePath) // remove temp symlink
		if err := os.Symlink(filepath.Join(dotfilesRepo, "source.txt"), targetFilePath); err != nil {
			t.Fatalf("Failed to set up correct symlink for test: %v", err)
		}

		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if err != nil {
			t.Errorf("SkipAction with correct symlink returned error: %v", err)
		}
	})

	// Case 2: Target is a regular file
	t.Run("TargetIsFile", func(t *testing.T) {
		createDummyFile(t, targetFilePath, "existing file content")
		defer os.Remove(targetFilePath)
		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if err != nil {
			t.Errorf("SkipAction with existing file returned error: %v", err)
		}
		// Check it's still the original file
		content, _ := os.ReadFile(targetFilePath)
		if string(content) != "existing file content" {
			t.Errorf("SkipAction modified the existing file content")
		}
	})

	// Case 3: Target is an incorrect symlink
	t.Run("TargetIsIncorrectSymlink", func(t *testing.T) {
		createDummyFile(t, filepath.Join(tempDir, "wrong_source.txt"), "wrong source")
		os.Symlink(filepath.Join(tempDir, "wrong_source.txt"), targetFilePath)
		defer os.Remove(targetFilePath)
		defer os.Remove(filepath.Join(tempDir, "wrong_source.txt"))

		df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
		err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
		if err != nil {
			t.Errorf("SkipAction with incorrect symlink returned error: %v", err)
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
	defer os.Remove(targetFilePath + ".bak") // Clean up backup

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	err := CreateSymlink(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false)
	if err != nil {
		t.Fatalf("BackupAction failed: %v", err)
	}

	// Check backup file
	backupContent, err := os.ReadFile(targetFilePath + ".bak")
	if err != nil {
		t.Fatalf("Could not read backup file: %v", err)
	}
	if string(backupContent) != "original target content" {
		t.Errorf("Backup content mismatch. Got '%s', want '%s'", string(backupContent), "original target content")
	}

	// Check new symlink
	linkDest, readErr := os.Readlink(targetFilePath)
	if readErr != nil {
		t.Fatalf("Could not read link at %s: %v", targetFilePath, readErr)
	}
	expandedSource, _ := config.ExpandPath(absoluteSourcePath)
	if linkDest != expandedSource {
		t.Errorf("Symlink at %s points to %s, expected %s", targetFilePath, linkDest, expandedSource)
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
		t.Errorf("Symlink at %s points to %s, expected %s after overwrite", targetFilePath, linkDest, expandedSource)
	}

	// Ensure original file (now a symlink) content reflects source if read through link
	linkedContent, err := os.ReadFile(targetFilePath)
	if err != nil {
		t.Fatalf("Could not read content through created symlink %s: %v", targetFilePath, err)
	}
	if string(linkedContent) != "new source for overwrite" {
		t.Errorf("Content read through symlink was '%s', expected '%s'", string(linkedContent), "new source for overwrite")
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
		t.Errorf("Symlink for absolute source at %s points to %s, expected %s", targetPath, linkDest, df.Source)
	}
}
