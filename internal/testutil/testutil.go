package testutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// WithHome redirects $HOME to a temp directory for test isolation.
// Returns the temp dir path. HOME is restored via t.Cleanup.
func WithHome(t *testing.T) string {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpDir, err := os.MkdirTemp("", "ralph-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
		os.RemoveAll(tmpDir)
	})
	return tmpDir
}

// EnsureRalphDir creates ~/.config/ralph/ under the given home dir.
// Returns the path to the ralph config dir.
func EnsureRalphDir(t *testing.T, homeDir string) string {
	t.Helper()
	dir := filepath.Join(homeDir, ".config", "ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create ralph dir: %v", err)
	}
	return dir
}

// RunGitCmd runs a git command in the given directory. Fails the test on error.
func RunGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2024-01-01T00:00:00",
		"GIT_COMMITTER_DATE=2024-01-01T00:00:00",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("git %s failed: %s, output: %s", strings.Join(args, " "), err, output)
	}
}

// InitGitRepo creates a git repository at dir with an initial commit.
// Returns the commit hash. Skips the test if git is unavailable.
func InitGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	RunGitCmd(t, dir, "init")
	RunGitCmd(t, dir, "config", "user.email", "test@test.com")
	RunGitCmd(t, dir, "config", "user.name", "Test")

	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	RunGitCmd(t, dir, "add", ".")
	RunGitCmd(t, dir, "commit", "-m", "initial")

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Skip("git not available or repo setup failed")
	}
	hash := strings.TrimSpace(string(out))
	if hash == "" {
		t.Skip("git not available or repo setup failed")
	}
	return hash
}

// SaveBuildStateJSON writes a build state as JSON to the standard location
// under the given home directory. The state is passed as an arbitrary value
// to avoid importing internal/hooks.
func SaveBuildStateJSON(t *testing.T, homeDir string, state any) {
	t.Helper()
	stateDir := EnsureRalphDir(t, homeDir)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".builds_state"), data, 0644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
}
