package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/dotfile"
	"github.com/mad01/ralph/internal/report"
)

func TestApplyDirsMirror_SymlinkDir(t *testing.T) {
	srcDir := t.TempDir()
	tgtDir := t.TempDir()
	repoDir := t.TempDir()

	// Create subdirectories in source (relative to repoDir)
	relSrc := "skills"
	absSrc := filepath.Join(repoDir, relSrc)
	os.MkdirAll(filepath.Join(absSrc, "skill-a"), 0755)
	os.MkdirAll(filepath.Join(absSrc, "skill-b"), 0755)
	// Create a hidden dir that should be skipped
	os.MkdirAll(filepath.Join(absSrc, ".hidden"), 0755)
	_ = srcDir // unused but kept for clarity

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"skills": {
				Source: relSrc,
				Target: tgtDir,
				Action: "symlink_dir",
			},
		},
	}

	var buf bytes.Buffer
	rpt := &report.Report{Command: "test"}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: "testhost",
		dryRun:      false,
		verbose:     true,
		w:           &buf,
		rpt:         rpt,
	}

	applyDirsMirror(ctx, dotfile.SymlinkActionBackup)

	// Verify symlinks were created
	for _, name := range []string{"skill-a", "skill-b"} {
		link := filepath.Join(tgtDir, name)
		info, err := os.Lstat(link)
		if err != nil {
			t.Errorf("expected symlink at %s, got error: %v", link, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink", link)
		}
		target, _ := os.Readlink(link)
		expected := filepath.Join(absSrc, name)
		if target != expected {
			t.Errorf("symlink %s points to %q, want %q", link, target, expected)
		}
	}

	// Verify hidden dir was skipped
	if _, err := os.Lstat(filepath.Join(tgtDir, ".hidden")); !os.IsNotExist(err) {
		t.Error("hidden directory should be skipped")
	}
}

func TestApplyDirsMirror_SymlinkFiles(t *testing.T) {
	repoDir := t.TempDir()
	tgtDir := t.TempDir()

	// Create files in source
	relSrc := "rc"
	absSrc := filepath.Join(repoDir, relSrc)
	os.MkdirAll(absSrc, 0755)
	os.WriteFile(filepath.Join(absSrc, "01-env.zsh"), []byte("# env"), 0644)
	os.WriteFile(filepath.Join(absSrc, "02-aliases.zsh"), []byte("# aliases"), 0644)
	os.WriteFile(filepath.Join(absSrc, ".hidden"), []byte("# hidden"), 0644)

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"zsh_rc": {
				Source: relSrc,
				Target: tgtDir,
				// Action defaults to "symlink"
			},
		},
	}

	var buf bytes.Buffer
	rpt := &report.Report{Command: "test"}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: "testhost",
		dryRun:      false,
		verbose:     true,
		w:           &buf,
		rpt:         rpt,
	}

	applyDirsMirror(ctx, dotfile.SymlinkActionBackup)

	for _, name := range []string{"01-env.zsh", "02-aliases.zsh"} {
		link := filepath.Join(tgtDir, name)
		info, err := os.Lstat(link)
		if err != nil {
			t.Errorf("expected symlink at %s, got error: %v", link, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink", link)
		}
	}

	// Hidden file should be skipped
	if _, err := os.Lstat(filepath.Join(tgtDir, ".hidden")); !os.IsNotExist(err) {
		t.Error("hidden file should be skipped")
	}
}

func TestApplyDirsMirror_SkipsDisabled(t *testing.T) {
	repoDir := t.TempDir()
	tgtDir := t.TempDir()

	relSrc := "src"
	os.MkdirAll(filepath.Join(repoDir, relSrc), 0755)
	os.WriteFile(filepath.Join(repoDir, relSrc, "file.txt"), []byte("x"), 0644)

	disabled := false
	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"disabled_mirror": {
				Source: relSrc,
				Target: tgtDir,
				Enable: &disabled,
			},
		},
	}

	var buf bytes.Buffer
	rpt := &report.Report{Command: "test"}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: "testhost",
		dryRun:      false,
		verbose:     true,
		w:           &buf,
		rpt:         rpt,
	}

	applyDirsMirror(ctx, dotfile.SymlinkActionBackup)

	// No symlinks should be created
	entries, _ := os.ReadDir(tgtDir)
	if len(entries) != 0 {
		t.Errorf("disabled mirror should create no symlinks, found %d entries", len(entries))
	}
}

func TestApplyDirsMirror_SkipsHostFiltered(t *testing.T) {
	repoDir := t.TempDir()
	tgtDir := t.TempDir()

	relSrc := "src"
	os.MkdirAll(filepath.Join(repoDir, relSrc), 0755)
	os.WriteFile(filepath.Join(repoDir, relSrc, "file.txt"), []byte("x"), 0644)

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"host_filtered": {
				Source: relSrc,
				Target: tgtDir,
				Hosts:  []string{"other-host"},
			},
		},
	}

	var buf bytes.Buffer
	rpt := &report.Report{Command: "test"}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: "testhost",
		dryRun:      false,
		verbose:     true,
		w:           &buf,
		rpt:         rpt,
	}

	applyDirsMirror(ctx, dotfile.SymlinkActionBackup)

	entries, _ := os.ReadDir(tgtDir)
	if len(entries) != 0 {
		t.Errorf("host-filtered mirror should create no symlinks, found %d entries", len(entries))
	}
}

func TestApplyDirsMirror_DryRun(t *testing.T) {
	repoDir := t.TempDir()
	tgtDir := t.TempDir()

	relSrc := "src"
	absSrc := filepath.Join(repoDir, relSrc)
	os.MkdirAll(absSrc, 0755)
	os.WriteFile(filepath.Join(absSrc, "file.txt"), []byte("x"), 0644)

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"test": {
				Source: relSrc,
				Target: tgtDir,
			},
		},
	}

	var buf bytes.Buffer
	rpt := &report.Report{Command: "test"}
	ctx := &applyContext{
		cfg:         cfg,
		currentHost: "testhost",
		dryRun:      true,
		verbose:     true,
		w:           &buf,
		rpt:         rpt,
	}

	applyDirsMirror(ctx, dotfile.SymlinkActionBackup)

	// In dry run, the target dir should have no symlinks
	entries, _ := os.ReadDir(tgtDir)
	if len(entries) != 0 {
		t.Errorf("dry run should not create symlinks, found %d entries", len(entries))
	}
}

func TestBuildIntendedManifest_TracksDirsMirror(t *testing.T) {
	repoDir := t.TempDir()

	// Create source directory with entries
	relSrc := "skills"
	absSrc := filepath.Join(repoDir, relSrc)
	os.MkdirAll(filepath.Join(absSrc, "skill-a"), 0755)
	os.MkdirAll(filepath.Join(absSrc, "skill-b"), 0755)
	os.MkdirAll(filepath.Join(absSrc, ".hidden"), 0755)

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"skills": {
				Source:      relSrc,
				Target:      "/tmp/test-skills",
				Action:      "symlink_dir",
				OwnerRecipe: "claude",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{Name: "claude", DeleteBehavior: "delete"},
		},
	}

	got, _ := buildIntendedManifest(cfg, "anyhost", time.Now())
	rec := got.Recipes["claude"]

	if len(rec.DirSymlinks) != 2 {
		t.Errorf("expected 2 dir_symlinks, got %d: %v", len(rec.DirSymlinks), rec.DirSymlinks)
	}

	// Check the hidden dir was not tracked
	for _, ds := range rec.DirSymlinks {
		if filepath.Base(ds) == ".hidden" {
			t.Error("hidden directory should not be tracked in manifest")
		}
	}
}

func TestBuildIntendedManifest_DirsMirrorSymlinkAction(t *testing.T) {
	repoDir := t.TempDir()

	relSrc := "rc"
	absSrc := filepath.Join(repoDir, relSrc)
	os.MkdirAll(absSrc, 0755)
	os.WriteFile(filepath.Join(absSrc, "file.zsh"), []byte("x"), 0644)

	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"rc": {
				Source:      relSrc,
				Target:      "/tmp/test-rc",
				OwnerRecipe: "shell",
				// Action defaults to "symlink"
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{Name: "shell", DeleteBehavior: "delete"},
		},
	}

	got, _ := buildIntendedManifest(cfg, "anyhost", time.Now())
	rec := got.Recipes["shell"]

	if len(rec.Symlinks) != 1 || rec.Symlinks[0] != "/tmp/test-rc/file.zsh" {
		t.Errorf("expected [/tmp/test-rc/file.zsh] in symlinks, got %v", rec.Symlinks)
	}
}

func TestBuildIntendedManifest_DirsMirrorSkipsDisabledAndHostFiltered(t *testing.T) {
	repoDir := t.TempDir()

	relSrc := "src"
	os.MkdirAll(filepath.Join(repoDir, relSrc), 0755)
	os.WriteFile(filepath.Join(repoDir, relSrc, "file.txt"), []byte("x"), 0644)

	disabled := false
	cfg := &config.Config{
		DotfilesRepoPath: repoDir,
		DirsMirror: map[string]config.DirMirror{
			"disabled": {
				Source:      relSrc,
				Target:      "/tmp/disabled",
				OwnerRecipe: "r1",
				Enable:      &disabled,
			},
			"host_filtered": {
				Source:      relSrc,
				Target:      "/tmp/host",
				OwnerRecipe: "r2",
				Hosts:       []string{"other-host"},
			},
			"no_owner": {
				Source: relSrc,
				Target: "/tmp/no-owner",
			},
		},
	}

	got, _ := buildIntendedManifest(cfg, "myhost", time.Now())

	if rec, ok := got.Recipes["r1"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("disabled item should not be tracked, got %v", rec.Symlinks)
	}
	if rec, ok := got.Recipes["r2"]; ok && len(rec.Symlinks) > 0 {
		t.Errorf("host-filtered item should not be tracked, got %v", rec.Symlinks)
	}
	if _, ok := got.Recipes["no_owner"]; ok {
		t.Error("item without owner recipe should not be tracked")
	}
}
