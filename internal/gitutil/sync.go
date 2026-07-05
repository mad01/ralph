package gitutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Clone clones url into target. If branch is non-empty it is passed to
// git clone -b (works for branches and tags). The "--" separator before the
// positional URL and target prevents a "-"-prefixed URL from being parsed as
// a git option (argument injection).
func Clone(w io.Writer, url, target, branch string) error {
	if err := runGitStreaming(w, "", cloneArgs(url, target, branch)...); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	return nil
}

// cloneArgs assembles the argv for `git clone`.
func cloneArgs(url, target, branch string) []string {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	return append(args, "--", url, target)
}

// Fetch fetches all remotes and tags in dir.
func Fetch(w io.Writer, dir string) error {
	if err := runGitStreaming(w, dir, "fetch", "--all", "--tags"); err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}
	return nil
}

// Checkout checks out a branch, tag, or commit in dir.
func Checkout(w io.Writer, dir, ref string) error {
	if err := runGitStreaming(w, dir, "checkout", ref); err != nil {
		return fmt.Errorf("failed to checkout %s: %w", ref, err)
	}
	return nil
}

// Pull pulls the latest changes in dir.
func Pull(w io.Writer, dir string) error {
	if err := runGitStreaming(w, dir, "pull"); err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}
	return nil
}

// CurrentBranch returns the checked-out branch name, or "" when HEAD is
// detached (tag or commit checkout) or dir is not a git repository.
func CurrentBranch(dir string) string {
	out, err := runGit(dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// ResolveRef resolves a ref (branch, tag, or commit) to its commit hash in
// dir. Returns "" if the ref is unknown or dir is not a git repository.
func ResolveRef(dir, ref string) string {
	out, err := runGit(dir, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// runGitStreaming runs a git subcommand with stdout streamed to w and stderr
// to the process stderr. dir may be empty for commands like clone.
func runGitStreaming(w io.Writer, dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
