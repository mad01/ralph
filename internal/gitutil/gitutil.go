package gitutil

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/execx"
)

// queryTimeout bounds a local git query (rev-parse, status, diff). These read
// the local repository only, so the bound is far below the network one; it is
// still generous because a timed-out query reads as "unknown" to callers. A
// var so tests can shorten it.
var queryTimeout = 2 * time.Minute

// networkTimeout bounds a git command that may reach a remote: clone, fetch,
// pull, and checkout (which can run filters that fetch). A var so tests can
// shorten it.
var networkTimeout = execx.DefaultTimeout

// GetGitHash returns the current git commit hash for a directory.
// Returns empty string if not a git repository or git is not available.
func GetGitHash(dir string) string {
	out, err := runGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// HasGitChanges checks if the working directory has uncommitted changes.
func HasGitChanges(dir string) bool {
	out, err := runGit(dir, "status", "--porcelain")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(out)) > 0
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

// runGit runs a local git query in dir and returns its stdout.
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	cmd := execx.TerminalCommand(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", timeoutError(args, queryTimeout)
	}
	return string(out), err
}

// timeoutError names the git subcommand that ran out of time and the limit.
func timeoutError(args []string, limit time.Duration) error {
	sub := "git"
	if len(args) > 0 {
		sub = "git " + args[0]
	}
	return fmt.Errorf("%s timed out after %s", sub, limit)
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

// SubtreeChangedSince reports whether the subtree rooted at dir changed
// between commit and HEAD. ok is false when there is no verdict — dir is
// outside a git repository, the commit is not present locally, the ref is
// unsafe, or git failed — and callers must not read changed in that case.
func SubtreeChangedSince(dir, commit string) (changed, ok bool) {
	if commit == "" || !IsSafeGitRef(commit) {
		return false, false
	}
	if _, err := runGit(dir, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return false, false
	}
	// The "." pathspec is relative to dir, which scopes the diff to dir's
	// subtree without path math (a root-relative prefix would be misread,
	// since git resolves pathspecs against the working directory).
	if _, err := runGit(dir, "diff", "--quiet", commit, "HEAD", "--", "."); err != nil {
		// diff --quiet exits 1 when the trees differ; anything else is a
		// git failure and yields no verdict.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, true
		}
		return false, false
	}
	return false, true
}
