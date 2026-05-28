package dotfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func TestCopyFile_SkipAction_ReturnsErrSkipped(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")
	createDummyFile(t, targetFilePath, "existing content")

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	err := CopyFile(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
	if !errors.Is(err, ErrSkipped) {
		t.Errorf("CopyFile SkipAction with existing target: want ErrSkipped, got %v", err)
	}

	content, _ := os.ReadFile(targetFilePath)
	if string(content) != "existing content" {
		t.Errorf("CopyFile SkipAction modified the existing file")
	}
}

func TestCopyFile_SkipAction_NoTarget_Copies(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "source content")

	targetFilePath := filepath.Join(tempDir, "target.txt")

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	err := CopyFile(io.Discard, df, dotfilesRepo, SymlinkActionSkip, false)
	if err != nil {
		t.Fatalf("CopyFile with no existing target returned error: %v", err)
	}

	content, _ := os.ReadFile(targetFilePath)
	if string(content) != "source content" {
		t.Errorf("CopyFile did not copy source content, got %q", string(content))
	}
}

func TestCopyFile_BackupAction_ExistingTarget(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "new content")

	targetFilePath := filepath.Join(tempDir, "target.txt")
	createDummyFile(t, targetFilePath, "old content")

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}
	err := CopyFile(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false)
	if err != nil {
		t.Fatalf("CopyFile BackupAction returned error: %v", err)
	}

	content, _ := os.ReadFile(targetFilePath)
	if string(content) != "new content" {
		t.Errorf("CopyFile BackupAction did not copy new content, got %q", string(content))
	}

	entries, _ := os.ReadDir(tempDir)
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bak" || len(e.Name()) > len("target.txt") && e.Name()[:10] == "target.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CopyFile BackupAction did not create a backup file")
	}
}

func TestCopyFile_BackupIsTimestampedAndDoesNotClobber(t *testing.T) {
	tempDir := t.TempDir()
	dotfilesRepo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "v1")

	targetFilePath := filepath.Join(tempDir, "target.txt")
	createDummyFile(t, targetFilePath, "original")

	df := config.Dotfile{Source: "source.txt", Target: targetFilePath}

	// First backup-copy: backs up "original".
	if err := CopyFile(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("first CopyFile returned error: %v", err)
	}
	// Change source, then backup-copy again: backs up "v1".
	createDummyFile(t, filepath.Join(dotfilesRepo, "source.txt"), "v2")
	if err := CopyFile(io.Discard, df, dotfilesRepo, SymlinkActionBackup, false); err != nil {
		t.Fatalf("second CopyFile returned error: %v", err)
	}

	// Backups must be timestamped (not a fixed ".bak") so the second run does
	// not clobber the first — both backups should coexist.
	baks, _ := filepath.Glob(targetFilePath + ".bak.*")
	if len(baks) < 2 {
		t.Fatalf("expected >=2 distinct timestamped backups, got %d: %v", len(baks), baks)
	}
	if _, err := os.Lstat(targetFilePath + ".bak"); err == nil {
		t.Errorf("copy backup should be timestamped, not a fixed .bak")
	}
	// One of the backups must still hold the original content.
	foundOriginal := false
	for _, b := range baks {
		if c, _ := os.ReadFile(b); string(c) == "original" {
			foundOriginal = true
		}
	}
	if !foundOriginal {
		t.Errorf("original content was clobbered; backups=%v", baks)
	}
}
