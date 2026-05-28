package gitutil

import (
	"os/exec"
	"strings"
)

// GetGitHash returns the current git commit hash for a directory.
// Returns empty string if not a git repository or git is not available.
func GetGitHash(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// HasGitChanges checks if the working directory has uncommitted changes.
func HasGitChanges(dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}
