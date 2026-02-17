package repo

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/mad01/ralph/internal/config"
)

// CloneOrUpdateRepo clones a git repository or updates it based on configuration.
// Behavior:
// - If target doesn't exist: clone (optionally checkout branch/commit)
// - If target exists and commit is set: fetch + checkout commit
// - If target exists and update=true: pull latest
// - Otherwise: skip
// If dryRun is true, it will only print the actions it would take.
func CloneOrUpdateRepo(w io.Writer, name string, repo config.Repo, dryRun bool) error {
	absoluteTarget, err := config.ExpandPath(repo.Target)
	if err != nil {
		return fmt.Errorf("failed to expand target path '%s': %w", repo.Target, err)
	}

	// Check if target directory exists
	info, err := os.Stat(absoluteTarget)
	targetExists := err == nil && info.IsDir()

	if !targetExists {
		// Clone the repository
		return cloneRepo(w, repo, absoluteTarget, dryRun)
	}

	// Target exists - check what action to take
	if repo.Commit != "" {
		// Pin to specific commit - fetch and checkout
		return checkoutCommit(w, repo, absoluteTarget, dryRun)
	}

	if repo.Update {
		// Pull latest
		return pullRepo(w, name, absoluteTarget, dryRun)
	}

	// No update or commit specified - skip
	fmt.Fprintf(w, "Repo '%s' already exists at '%s'. Skipping.\n", name, absoluteTarget)
	return nil
}

// cloneRepo clones a git repository to the target path.
func cloneRepo(w io.Writer, repo config.Repo, absoluteTarget string, dryRun bool) error {
	args := []string{"clone"}

	if repo.Branch != "" {
		args = append(args, "-b", repo.Branch)
	}

	args = append(args, repo.URL, absoluteTarget)

	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would clone: git %v\n", args)
		if repo.Commit != "" {
			fmt.Fprintf(w, "[DRY RUN] Would checkout commit: %s\n", repo.Commit)
		}
		return nil
	}

	fmt.Fprintf(w, "Cloning: git %v\n", args)
	cmd := exec.Command("git", args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// If commit is specified, checkout that commit after cloning
	if repo.Commit != "" {
		fmt.Fprintf(w, "Checking out commit: %s\n", repo.Commit)
		checkoutCmd := exec.Command("git", "checkout", repo.Commit)
		checkoutCmd.Dir = absoluteTarget
		checkoutCmd.Stdout = w
		checkoutCmd.Stderr = os.Stderr
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout commit %s: %w", repo.Commit, err)
		}
	}

	return nil
}

// checkoutCommit fetches and checks out a specific commit.
func checkoutCommit(w io.Writer, repo config.Repo, absoluteTarget string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would fetch and checkout commit %s in '%s'\n", repo.Commit, absoluteTarget)
		return nil
	}

	fmt.Fprintf(w, "Fetching in '%s'...\n", absoluteTarget)
	fetchCmd := exec.Command("git", "fetch", "--all")
	fetchCmd.Dir = absoluteTarget
	fetchCmd.Stdout = w
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	fmt.Fprintf(w, "Checking out commit: %s\n", repo.Commit)
	checkoutCmd := exec.Command("git", "checkout", repo.Commit)
	checkoutCmd.Dir = absoluteTarget
	checkoutCmd.Stdout = w
	checkoutCmd.Stderr = os.Stderr
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit %s: %w", repo.Commit, err)
	}

	return nil
}

// pullRepo pulls the latest changes in the repository.
func pullRepo(w io.Writer, name string, absoluteTarget string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(w, "[DRY RUN] Would pull latest in '%s'\n", absoluteTarget)
		return nil
	}

	fmt.Fprintf(w, "Pulling latest for '%s' in '%s'...\n", name, absoluteTarget)
	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = absoluteTarget
	pullCmd.Stdout = w
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}

// ProcessRepos processes all configured repositories.
func ProcessRepos(w io.Writer, repos map[string]config.Repo, currentHost string, dryRun bool) error {
	if len(repos) == 0 {
		return nil
	}

	fmt.Fprintln(w, "\nProcessing repositories...")
	for name, repo := range repos {
		if !config.IsEnabled(repo.Enable) {
			fmt.Fprintf(w, "  Skipping repo: %s (disabled)\n", name)
			continue
		}
		if !config.ShouldApplyForHost(repo.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping repo: %s (host filter)\n", name)
			continue
		}
		fmt.Fprintf(w, "  Repo: %s (URL: %s)\n", name, repo.URL)
		if err := CloneOrUpdateRepo(w, name, repo, dryRun); err != nil {
			return fmt.Errorf("repo '%s' failed: %w", name, err)
		}
	}
	return nil
}
