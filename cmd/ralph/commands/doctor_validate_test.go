package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/report"
)

func createTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("Failed to create parent dirs for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}

func TestValidateDotfileTarget_CopyAction(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "repo")

	t.Run("FileExists", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "source.txt"), "content")
		targetPath := filepath.Join(tempDir, "target.txt")
		createTestFile(t, targetPath, "content")

		df := config.Dotfile{Source: "source.txt", Target: targetPath, Action: "copy"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusOK {
			t.Errorf("copy with existing file: want StatusOK, got %v", status)
		}
	})

	t.Run("FileMissing", func(t *testing.T) {
		targetPath := filepath.Join(tempDir, "missing.txt")

		df := config.Dotfile{Source: "source.txt", Target: targetPath, Action: "copy"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("copy with missing file: want StatusWarn, got %v", status)
		}
	})

	t.Run("IsSymlinkNotFile", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "source.txt"), "content")
		targetPath := filepath.Join(tempDir, "symlink-target.txt")
		realFile := filepath.Join(tempDir, "real-file.txt")
		createTestFile(t, realFile, "content")
		if err := os.Symlink(realFile, targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "source.txt", Target: targetPath, Action: "copy"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("copy with symlink target: want StatusWarn, got %v", status)
		}
	})

	t.Run("SizeMismatch", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "source.txt"), "short")
		targetPath := filepath.Join(tempDir, "size-mismatch.txt")
		createTestFile(t, targetPath, "this is much longer content")

		df := config.Dotfile{Source: "source.txt", Target: targetPath, Action: "copy"}
		status, msg, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("copy with size mismatch: want StatusWarn, got %v", status)
		}
		if msg == "" {
			t.Error("copy with size mismatch: expected non-empty message")
		}
	})

	t.Run("TemplateSkipsSizeCheck", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "tmpl.txt"), "{{ .short }}")
		targetPath := filepath.Join(tempDir, "template-copy.txt")
		createTestFile(t, targetPath, "this is the rendered output which is longer")

		df := config.Dotfile{
			Source:     "tmpl.txt",
			Target:     targetPath,
			Action:     "copy",
			IsTemplate: true,
		}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusOK {
			t.Errorf("template copy: want StatusOK, got %v", status)
		}
	})
}

func TestValidateDotfileTarget_SymlinkDirAction(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "repo")

	t.Run("CorrectDirSymlink", func(t *testing.T) {
		sourceDir := filepath.Join(repoPath, "mydir")
		os.MkdirAll(sourceDir, 0o755)
		targetPath := filepath.Join(tempDir, "link-to-mydir")
		if err := os.Symlink(sourceDir, targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "mydir", Target: targetPath, Action: "symlink_dir"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusOK {
			t.Errorf("correct dir symlink: want StatusOK, got %v", status)
		}
	})

	t.Run("NotSymlink", func(t *testing.T) {
		sourceDir := filepath.Join(repoPath, "mydir2")
		os.MkdirAll(sourceDir, 0o755)
		targetPath := filepath.Join(tempDir, "actual-dir")
		os.MkdirAll(targetPath, 0o755)
		defer os.RemoveAll(targetPath)

		df := config.Dotfile{Source: "mydir2", Target: targetPath, Action: "symlink_dir"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("dir not a symlink: want StatusWarn, got %v", status)
		}
	})

	t.Run("WrongTarget", func(t *testing.T) {
		sourceDir := filepath.Join(repoPath, "mydir3")
		os.MkdirAll(sourceDir, 0o755)
		wrongDir := filepath.Join(tempDir, "wrong-dir")
		os.MkdirAll(wrongDir, 0o755)
		targetPath := filepath.Join(tempDir, "link-to-wrong")
		if err := os.Symlink(wrongDir, targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "mydir3", Target: targetPath, Action: "symlink_dir"}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("dir symlink to wrong target: want StatusWarn, got %v", status)
		}
	})
}

func TestValidateDotfileTarget_SymlinkAction(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "repo")

	t.Run("CorrectSymlink", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "source.txt"), "content")
		targetPath := filepath.Join(tempDir, "correct-link.txt")
		if err := os.Symlink(filepath.Join(repoPath, "source.txt"), targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "source.txt", Target: targetPath}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusOK {
			t.Errorf("correct symlink: want StatusOK, got %v", status)
		}
	})

	t.Run("BrokenSymlink", func(t *testing.T) {
		targetPath := filepath.Join(tempDir, "broken-link.txt")
		if err := os.Symlink(filepath.Join(repoPath, "nonexistent.txt"), targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "nonexistent.txt", Target: targetPath}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusFail {
			t.Errorf("broken symlink: want StatusFail, got %v", status)
		}
	})

	t.Run("NotASymlink", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "source.txt"), "content")
		targetPath := filepath.Join(tempDir, "not-a-symlink.txt")
		createTestFile(t, targetPath, "content")

		df := config.Dotfile{Source: "source.txt", Target: targetPath}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("not a symlink: want StatusWarn, got %v", status)
		}
	})

	t.Run("TargetDoesNotExist", func(t *testing.T) {
		targetPath := filepath.Join(tempDir, "no-target.txt")

		df := config.Dotfile{Source: "source.txt", Target: targetPath}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusWarn {
			t.Errorf("target does not exist: want StatusWarn, got %v", status)
		}
	})

	t.Run("TemplateSymlink", func(t *testing.T) {
		createTestFile(t, filepath.Join(repoPath, "tmpl.txt"), "template")
		processedPath := filepath.Join(tempDir, "processed-tmpl.txt")
		createTestFile(t, processedPath, "rendered")
		targetPath := filepath.Join(tempDir, "template-link.txt")
		if err := os.Symlink(processedPath, targetPath); err != nil {
			t.Fatalf("Failed to create symlink: %v", err)
		}
		t.Cleanup(func() { os.Remove(targetPath) })

		df := config.Dotfile{Source: "tmpl.txt", Target: targetPath, IsTemplate: true}
		status, _, _ := validateDotfileTarget(df, targetPath, repoPath)
		if status != report.StatusOK {
			t.Errorf("template symlink: want StatusOK, got %v", status)
		}
	})
}
