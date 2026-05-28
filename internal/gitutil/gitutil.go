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

// GetTreeHash returns the git tree object hash for the subtree rooted at dir,
// at the current HEAD. Unlike GetGitHash (which returns the repo-wide commit
// hash), this only changes when files within dir's subtree change — commits
// elsewhere in the repository leave it untouched. Returns empty string if dir
// is not inside a git repository or git is unavailable.
func GetTreeHash(dir string) string {
	// --show-prefix yields dir's path relative to the repo work-tree root,
	// computed by git itself — avoiding manual path math that breaks when the
	// path contains symlinks (e.g. /var -> /private/var on macOS). It always
	// uses forward slashes and has a trailing slash (empty at the root).
	prefixOut, err := runGit(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return ""
	}
	prefix := strings.TrimSuffix(strings.TrimSpace(prefixOut), "/")

	revArg := "HEAD^{tree}"
	if prefix != "" {
		revArg = "HEAD:" + prefix
	}

	out, err := runGit(dir, "rev-parse", revArg)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// HasGitChangesInPath reports whether dir's subtree has uncommitted changes
// (modified, staged, or untracked-but-not-ignored). It is scoped to dir: a
// change elsewhere in the repository does not count, and gitignored paths
// (build output, compiled binaries) are excluded by default. Returns false if
// dir is not inside a git repository or git is unavailable.
func HasGitChangesInPath(dir string) bool {
	// The "." pathspec is resolved relative to cmd.Dir, restricting the status
	// to dir's subtree. Ignored files are omitted unless --ignored is passed.
	out, err := runGit(dir, "status", "--porcelain", "--", ".")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(out)) > 0
}

// runGit runs a git subcommand in dir and returns its stdout.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
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
