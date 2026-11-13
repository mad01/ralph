package dotfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func mustSymlink(t *testing.T, dest, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dest, link); err != nil {
		t.Fatal(err)
	}
}

func assertLinksTo(t *testing.T, link, want string) {
	t.Helper()
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("%s is not a symlink: %v", link, err)
	}
	if got != want {
		t.Errorf("%s points to %s, want %s", link, got, want)
	}
}

// backupsOf returns the backups ralph made next to target.
func backupsOf(t *testing.T, target string) []string {
	t.Helper()
	matches, err := filepath.Glob(target + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

// failSymlink makes every link ralph creates fail until the test ends.
func failSymlink(t *testing.T) {
	t.Helper()
	orig := osSymlink
	osSymlink = func(string, string) error { return errors.New("symlink refused") }
	t.Cleanup(func() { osSymlink = orig })
}

// When linking fails after the existing target was moved to its backup, the
// original must be back in place and the error returned.
func TestCreateSymlink_FailedLinkRestoresOriginal(t *testing.T) {
	tempDir := t.TempDir()
	repo := filepath.Join(tempDir, "repo")
	createDummyFile(t, filepath.Join(repo, "gitconfig"), "ralph's gitconfig")
	target := filepath.Join(tempDir, "home", ".gitconfig")
	createDummyFile(t, target, "user data")
	failSymlink(t)

	df := config.Dotfile{Source: "gitconfig", Target: target}
	if err := CreateSymlink(io.Discard, df, repo, SymlinkActionBackup, false); err == nil {
		t.Fatal("want the link error, got nil")
	}

	got, err := os.ReadFile(target)
	if err != nil || string(got) != "user data" {
		t.Errorf("original target not restored: content %q, err %v", got, err)
	}
	if baks := backupsOf(t, target); len(baks) != 0 {
		t.Errorf("backup left behind after restore: %v", baks)
	}
}

func TestCreateDirSymlink_FailedLinkRestoresOriginal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action SymlinkAction
	}{
		{"backup", SymlinkActionBackup},
		{"overwrite non-empty dir", SymlinkActionOverwrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			repo := filepath.Join(tempDir, "repo")
			if err := os.MkdirAll(filepath.Join(repo, "nvim"), 0o755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(tempDir, "home", ".config", "nvim")
			userFile := filepath.Join(target, "init.lua")
			createDummyFile(t, userFile, "user data")
			failSymlink(t)

			df := config.Dotfile{Source: "nvim", Target: target}
			if err := CreateDirSymlink(io.Discard, df, repo, tc.action, false); err == nil {
				t.Fatal("want the link error, got nil")
			}

			got, err := os.ReadFile(userFile)
			if err != nil || string(got) != "user data" {
				t.Errorf("original directory not restored: content %q, err %v", got, err)
			}
			if baks := backupsOf(t, target); len(baks) != 0 {
				t.Errorf("backup left behind after restore: %v", baks)
			}
		})
	}
}

// The user's own symlink at the target is backed up, and that backup must
// outlive both the call that made it and the next "already linked" run. A
// redundant backup of ralph's own link (pointing at the source) is still
// reaped.
func TestCreateSymlink_BackupOfForeignSymlinkSurvives(t *testing.T) {
	tempDir := t.TempDir()
	repo := filepath.Join(tempDir, "repo")
	source := filepath.Join(repo, "gitconfig")
	createDummyFile(t, source, "ralph's gitconfig")
	userOwn := filepath.Join(tempDir, "elsewhere", "gitconfig")
	createDummyFile(t, userOwn, "user's gitconfig")

	target := filepath.Join(tempDir, "home", ".gitconfig")
	mustSymlink(t, userOwn, target)
	mustSymlink(t, source, target+".bak.20260525T191256.068349000")

	df := config.Dotfile{Source: "gitconfig", Target: target}
	for run := 1; run <= 2; run++ {
		if err := CreateSymlink(io.Discard, df, repo, SymlinkActionBackup, false); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		assertLinksTo(t, target, source)
		baks := backupsOf(t, target)
		if len(baks) != 1 {
			t.Fatalf("run %d: want only the backup of the user's link, got %v", run, baks)
		}
		assertLinksTo(t, baks[0], userOwn)
	}
}

func TestCreateDirSymlink_BackupOfForeignSymlinkSurvives(t *testing.T) {
	tempDir := t.TempDir()
	repo := filepath.Join(tempDir, "repo")
	sourceDir := filepath.Join(repo, "nvim")
	userDir := filepath.Join(tempDir, "elsewhere", "nvim")
	for _, d := range []string{sourceDir, userDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	target := filepath.Join(tempDir, "home", ".config", "nvim")
	mustSymlink(t, userDir, target)
	mustSymlink(t, sourceDir, target+".bak.20260525T191256.068349000")

	df := config.Dotfile{Source: "nvim", Target: target}
	for run := 1; run <= 2; run++ {
		if err := CreateDirSymlink(io.Discard, df, repo, SymlinkActionBackup, false); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		assertLinksTo(t, target, sourceDir)
		baks := backupsOf(t, target)
		if len(baks) != 1 {
			t.Fatalf("run %d: want only the backup of the user's link, got %v", run, baks)
		}
		assertLinksTo(t, baks[0], userDir)
	}
}
