package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/ralph/internal/config"
)

func setupTestEnv(t *testing.T) (tempDir string, cleanup func()) {
	t.Helper()
	tempDir = t.TempDir()
	return tempDir, func() {}
}

func TestCheckMigration_AlreadyCorrect(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo structure
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "editors", "nvim"), 0755)
	os.WriteFile(filepath.Join(repoPath, "editors", "nvim", "init.lua"), []byte("-- nvim config"), 0644)

	// Create target directory and symlink
	targetDir := filepath.Join(tempDir, "target")
	os.MkdirAll(targetDir, 0755)
	targetPath := filepath.Join(targetDir, "init.lua")
	sourcePath := filepath.Join(repoPath, "editors", "nvim", "init.lua")
	os.Symlink(sourcePath, targetPath)

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"nvim_init": {
				Source: "editors/nvim/init.lua",
				Target: targetPath,
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{
				Dir: "editors",
				LegacyPaths: map[string]string{
					"dotter_files/nvim/init.lua": "nvim/init.lua",
				},
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.AlreadyOK != 1 {
		t.Errorf("Expected 1 already correct, got %d", plan.AlreadyOK)
	}
	if plan.NeedsUpdate != 0 {
		t.Errorf("Expected 0 needs update, got %d", plan.NeedsUpdate)
	}
}

func TestCheckMigration_NeedsUpdate(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo with both old and new structure
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "editors", "nvim"), 0755)
	os.MkdirAll(filepath.Join(repoPath, "dotter_files", "nvim"), 0755)

	// Create file in new location
	newFile := filepath.Join(repoPath, "editors", "nvim", "init.lua")
	os.WriteFile(newFile, []byte("-- nvim config"), 0644)

	// Create symlink pointing to old location (which would be broken)
	oldFile := filepath.Join(repoPath, "dotter_files", "nvim", "init.lua")
	// Don't create old file - simulate it was moved

	targetDir := filepath.Join(tempDir, "target")
	os.MkdirAll(targetDir, 0755)
	targetPath := filepath.Join(targetDir, "init.lua")
	os.Symlink(oldFile, targetPath) // Points to non-existent old path

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"nvim_init": {
				Source: "editors/nvim/init.lua",
				Target: targetPath,
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{
				Dir: "editors",
				LegacyPaths: map[string]string{
					"dotter_files/nvim/init.lua": "nvim/init.lua",
				},
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.NeedsUpdate != 1 {
		t.Errorf("Expected 1 needs update, got %d", plan.NeedsUpdate)
	}

	// Verify the result details
	if len(plan.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(plan.Results))
	}
	result := plan.Results[0]
	if result.Status != StatusNeedsUpdate {
		t.Errorf("Expected StatusNeedsUpdate, got %v", result.Status)
	}
	if result.CurrentSource != oldFile {
		t.Errorf("CurrentSource = %q, want %q", result.CurrentSource, oldFile)
	}
	if result.NewSource != newFile {
		t.Errorf("NewSource = %q, want %q", result.NewSource, newFile)
	}
}

func TestCheckMigration_NotExist(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(repoPath, 0755)

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"newfile": {
				Source: "new/file.txt",
				Target: filepath.Join(tempDir, "nonexistent", "file.txt"),
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.NotExist != 1 {
		t.Errorf("Expected 1 not exist, got %d", plan.NotExist)
	}
}

func TestCheckMigration_NotSymlink(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(repoPath, 0755)

	// Create a regular file instead of a symlink
	targetPath := filepath.Join(tempDir, "regular_file.txt")
	os.WriteFile(targetPath, []byte("regular file"), 0644)

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"file": {
				Source: "file.txt",
				Target: targetPath,
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.NotSymlinks != 1 {
		t.Errorf("Expected 1 not symlink, got %d", plan.NotSymlinks)
	}
}

func TestCheckMigration_BrokenNoLegacy(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(repoPath, 0755)

	// Create symlink pointing to non-existent file
	targetPath := filepath.Join(tempDir, "broken_link")
	os.Symlink("/nonexistent/path/file.txt", targetPath)

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"file": {
				Source: "file.txt",
				Target: targetPath,
			},
		},
		// No legacy paths configured
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.Broken != 1 {
		t.Errorf("Expected 1 broken, got %d", plan.Broken)
	}
}

func TestExecuteMigration_DryRun(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "new"), 0755)
	newFile := filepath.Join(repoPath, "new", "file.txt")
	os.WriteFile(newFile, []byte("content"), 0644)

	// Create symlink pointing to old location
	oldFile := filepath.Join(repoPath, "old", "file.txt")
	targetPath := filepath.Join(tempDir, "target.txt")
	os.Symlink(oldFile, targetPath)

	plan := &MigrationPlan{
		RepoPath: repoPath,
		Results: []MigrationResult{
			{
				Target:        targetPath,
				CurrentSource: oldFile,
				NewSource:     newFile,
				Status:        StatusNeedsUpdate,
			},
		},
		NeedsUpdate: 1,
	}

	// Execute with dry run
	err := ExecuteMigration(plan, true)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}

	// Verify symlink was NOT changed
	link, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if link != oldFile {
		t.Errorf("Dry run should not change symlink, got %q, want %q", link, oldFile)
	}
}

func TestExecuteMigration_Actual(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "new"), 0755)
	newFile := filepath.Join(repoPath, "new", "file.txt")
	os.WriteFile(newFile, []byte("content"), 0644)

	// Create symlink pointing to old location
	oldFile := filepath.Join(repoPath, "old", "file.txt")
	targetPath := filepath.Join(tempDir, "target.txt")
	os.Symlink(oldFile, targetPath)

	plan := &MigrationPlan{
		RepoPath: repoPath,
		Results: []MigrationResult{
			{
				Target:        targetPath,
				CurrentSource: oldFile,
				NewSource:     newFile,
				Status:        StatusNeedsUpdate,
			},
		},
		NeedsUpdate: 1,
	}

	// Execute actual migration
	err := ExecuteMigration(plan, false)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}

	// Verify symlink was changed
	link, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if link != newFile {
		t.Errorf("Symlink should be updated, got %q, want %q", link, newFile)
	}
}

func TestExecuteMigration_Idempotent(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "new"), 0755)
	newFile := filepath.Join(repoPath, "new", "file.txt")
	os.WriteFile(newFile, []byte("content"), 0644)

	// Create symlink already pointing to correct location
	targetPath := filepath.Join(tempDir, "target.txt")
	os.Symlink(newFile, targetPath)

	plan := &MigrationPlan{
		RepoPath: repoPath,
		Results: []MigrationResult{
			{
				Target:        targetPath,
				CurrentSource: newFile,
				NewSource:     newFile,
				Status:        StatusAlreadyCorrect, // Not StatusNeedsUpdate
			},
		},
		AlreadyOK: 1,
	}

	// Execute migration (should be a no-op)
	err := ExecuteMigration(plan, false)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}

	// Verify symlink is still correct
	link, err := os.Readlink(targetPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if link != newFile {
		t.Errorf("Symlink should still be correct, got %q, want %q", link, newFile)
	}
}

func TestMigrationStatus_String(t *testing.T) {
	tests := []struct {
		status   MigrationStatus
		expected string
	}{
		{StatusAlreadyCorrect, "CORRECT"},
		{StatusNeedsUpdate, "UPDATE"},
		{StatusBroken, "BROKEN"},
		{StatusNotSymlink, "NOT_SYMLINK"},
		{StatusNotExist, "NOT_EXIST"},
		{StatusError, "ERROR"},
		{MigrationStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("MigrationStatus(%d).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func TestCheckMigration_DirectorySymlink(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo with directory
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "editors", "nvim", "lua"), 0755)
	os.WriteFile(filepath.Join(repoPath, "editors", "nvim", "lua", "init.lua"), []byte("-- lua"), 0644)

	// Create directory symlink pointing to old location
	oldDir := filepath.Join(repoPath, "dotter_files", "nvim")
	targetPath := filepath.Join(tempDir, "config", "nvim")
	os.MkdirAll(filepath.Dir(targetPath), 0755)
	os.Symlink(oldDir, targetPath) // Points to non-existent old path

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"nvim_dir": {
				Source: "editors/nvim",
				Target: targetPath,
				Action: "symlink_dir",
			},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{
				Dir: "editors",
				LegacyPaths: map[string]string{
					"dotter_files/nvim": "nvim",
				},
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.NeedsUpdate != 1 {
		t.Errorf("Expected 1 needs update for directory symlink, got %d", plan.NeedsUpdate)
	}
}

func TestCheckMigration_MixedState(t *testing.T) {
	tempDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create dotfiles repo
	repoPath := filepath.Join(tempDir, "dotfiles")
	os.MkdirAll(filepath.Join(repoPath, "editors"), 0755)

	// File 1: Already correct
	file1New := filepath.Join(repoPath, "editors", "file1.txt")
	os.WriteFile(file1New, []byte("file1"), 0644)
	target1 := filepath.Join(tempDir, "file1.txt")
	os.Symlink(file1New, target1)

	// File 2: Needs update (points to old location)
	file2Old := filepath.Join(repoPath, "old", "file2.txt")
	file2New := filepath.Join(repoPath, "editors", "file2.txt")
	os.WriteFile(file2New, []byte("file2"), 0644)
	target2 := filepath.Join(tempDir, "file2.txt")
	os.Symlink(file2Old, target2)

	// File 3: Not exist yet
	target3 := filepath.Join(tempDir, "nonexistent", "file3.txt")

	cfg := &config.Config{
		DotfilesRepoPath: repoPath,
		Dotfiles: map[string]config.Dotfile{
			"file1": {Source: "editors/file1.txt", Target: target1},
			"file2": {Source: "editors/file2.txt", Target: target2},
			"file3": {Source: "editors/file3.txt", Target: target3},
		},
		LoadedRecipes: []config.LoadedRecipeInfo{
			{
				Dir: "editors",
				LegacyPaths: map[string]string{
					"old/file2.txt": "file2.txt",
				},
			},
		},
	}

	plan, err := CheckMigration(cfg)
	if err != nil {
		t.Fatalf("CheckMigration() error: %v", err)
	}

	if plan.AlreadyOK != 1 {
		t.Errorf("Expected 1 already OK, got %d", plan.AlreadyOK)
	}
	if plan.NeedsUpdate != 1 {
		t.Errorf("Expected 1 needs update, got %d", plan.NeedsUpdate)
	}
	if plan.NotExist != 1 {
		t.Errorf("Expected 1 not exist, got %d", plan.NotExist)
	}
}
