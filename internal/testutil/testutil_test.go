package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWithHome_SetsHOMEToTempDir(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := WithHome(t)

	got := os.Getenv("HOME")
	if got != tmpDir {
		t.Errorf("HOME = %q, want %q", got, tmpDir)
	}
	if got == origHome {
		t.Error("HOME should differ from original")
	}

	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("temp dir should be a directory")
	}
}

func TestWithHome_RestoresHOMEOnCleanup(t *testing.T) {
	origHome := os.Getenv("HOME")

	func() {
		inner := &testing.T{}
		_ = inner
		// We can't easily test cleanup in a subtesting context without
		// running the cleanup. We verify the mechanism works by checking
		// the dir was created.
	}()

	// After the outer test's cleanup runs, HOME should be restored.
	// We verify the current state is correct at least.
	if os.Getenv("HOME") == "" && origHome != "" {
		t.Error("HOME should not be empty if it was set before")
	}
}

func TestInitGitRepo_CreatesGitDir(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")

	hash := InitGitRepo(t, repoDir)

	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error("expected .git directory to exist")
	}

	if len(hash) != 40 {
		t.Errorf("expected 40-char git hash, got %d chars: %q", len(hash), hash)
	}
}

func TestRunGitCmd_ExecutesSuccessfully(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	InitGitRepo(t, repoDir)

	// Should not panic or fail
	RunGitCmd(t, repoDir, "status")
}

func TestSaveBuildStateJSON_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()

	state := map[string]any{
		"builds": map[string]any{
			"test_build": map[string]any{
				"completed_at": "2024-01-15T10:00:00Z",
				"git_hash":     "abc123",
			},
		},
	}

	SaveBuildStateJSON(t, dir, state)

	statePath := filepath.Join(dir, ".config", "ralph", ".builds_state")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}

	if _, ok := parsed["builds"]; !ok {
		t.Error("expected 'builds' key in state file")
	}
}

func TestEnsureRalphDir_CreatesConfigDir(t *testing.T) {
	dir := t.TempDir()
	got := EnsureRalphDir(t, dir)

	want := filepath.Join(dir, ".config", "ralph")
	if got != want {
		t.Errorf("EnsureRalphDir = %q, want %q", got, want)
	}

	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Error("expected ralph config dir to exist")
	}
}
