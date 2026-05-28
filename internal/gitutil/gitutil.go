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

// dangerousURLPrefixes are git transport schemes that can execute arbitrary
// commands and must never be accepted from configuration.
var dangerousURLPrefixes = []string{"ext::", "fd::"}

// IsSafeRemoteURL reports whether a git remote URL is safe to pass to git from
// configuration. It rejects empty URLs, option injection (a leading "-"), and
// transports that can run arbitrary commands (ext::, fd::).
func IsSafeRemoteURL(url string) bool {
	if url == "" || strings.HasPrefix(url, "-") {
		return false
	}
	lower := strings.ToLower(url)
	for _, p := range dangerousURLPrefixes {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	return true
}

// IsSafeGitRef reports whether a branch/commit/ref is safe to pass to git as a
// positional argument — i.e. it won't be misparsed as an option. Empty refs
// are considered safe (callers omit them).
func IsSafeGitRef(ref string) bool {
	return !strings.HasPrefix(ref, "-")
}
