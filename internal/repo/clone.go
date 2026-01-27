package repo

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mad01/dotter/internal/config"
)

// CloneOrUpdateRepo clones a git repository or updates it based on configuration.
// Behavior:
// - If target doesn't exist: clone (optionally checkout branch/commit)
// - If target exists and commit is set: fetch + checkout commit
// - If target exists and update=true: pull latest
// - Otherwise: skip
// If dryRun is true, it will only print the actions it would take.
func CloneOrUpdateRepo(name string, repo config.Repo, dryRun bool) error {
	absoluteTarget, err := config.ExpandPath(repo.Target)
	if err != nil {
		return fmt.Errorf("failed to expand target path '%s': %w", repo.Target, err)
	}

	// Check if target directory exists
	info, err := os.Stat(absoluteTarget)
	targetExists := err == nil && info.IsDir()

	if !targetExists {
		// Clone the repository
		return cloneRepo(repo, absoluteTarget, dryRun)
	}

	// Target exists - check what action to take
	if repo.Commit != "" {
		// Pin to specific commit - fetch and checkout
		return checkoutCommit(repo, absoluteTarget, dryRun)
	}

	if repo.Update {
		// Pull latest
		return pullRepo(name, absoluteTarget, dryRun)
	}

	// No update or commit specified - skip
	fmt.Printf("Repo '%s' already exists at '%s'. Skipping.\n", name, absoluteTarget)
	return nil
}

// cloneRepo clones a git repository to the target path.
func cloneRepo(repo config.Repo, absoluteTarget string, dryRun bool) error {
	args := []string{"clone"}

	if repo.Branch != "" {
		args = append(args, "-b", repo.Branch)
	}

	args = append(args, repo.URL, absoluteTarget)

	if dryRun {
		fmt.Printf("[DRY RUN] Would clone: git %v\n", args)
		if repo.Commit != "" {
			fmt.Printf("[DRY RUN] Would checkout commit: %s\n", repo.Commit)
		}
		return nil
	}

	fmt.Printf("Cloning: git %v\n", args)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// If commit is specified, checkout that commit after cloning
	if repo.Commit != "" {
		fmt.Printf("Checking out commit: %s\n", repo.Commit)
		checkoutCmd := exec.Command("git", "checkout", repo.Commit)
		checkoutCmd.Dir = absoluteTarget
		checkoutCmd.Stdout = os.Stdout
		checkoutCmd.Stderr = os.Stderr
		if err := checkoutCmd.Run(); err != nil {
			return fmt.Errorf("failed to checkout commit %s: %w", repo.Commit, err)
		}
	}

	return nil
}

// checkoutCommit fetches and checks out a specific commit.
func checkoutCommit(repo config.Repo, absoluteTarget string, dryRun bool) error {
	if dryRun {
		fmt.Printf("[DRY RUN] Would fetch and checkout commit %s in '%s'\n", repo.Commit, absoluteTarget)
		return nil
	}

	fmt.Printf("Fetching in '%s'...\n", absoluteTarget)
	fetchCmd := exec.Command("git", "fetch", "--all")
	fetchCmd.Dir = absoluteTarget
	fetchCmd.Stdout = os.Stdout
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	fmt.Printf("Checking out commit: %s\n", repo.Commit)
	checkoutCmd := exec.Command("git", "checkout", repo.Commit)
	checkoutCmd.Dir = absoluteTarget
	checkoutCmd.Stdout = os.Stdout
	checkoutCmd.Stderr = os.Stderr
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit %s: %w", repo.Commit, err)
	}

	return nil
}

// pullRepo pulls the latest changes in the repository.
func pullRepo(name string, absoluteTarget string, dryRun bool) error {
	if dryRun {
		fmt.Printf("[DRY RUN] Would pull latest in '%s'\n", absoluteTarget)
		return nil
	}

	fmt.Printf("Pulling latest for '%s' in '%s'...\n", name, absoluteTarget)
	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = absoluteTarget
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull: %w", err)
	}

	return nil
}

// ProcessRepos processes all configured repositories.
func ProcessRepos(repos map[string]config.Repo, currentHost string, dryRun bool) error {
	if len(repos) == 0 {
		return nil
	}

	fmt.Println("\nProcessing repositories...")
	for name, repo := range repos {
		if !config.ShouldApplyForHost(repo.Hosts, currentHost) {
			fmt.Printf("  Skipping repo: %s (host filter)\n", name)
			continue
		}
		fmt.Printf("  Repo: %s (URL: %s)\n", name, repo.URL)
		if err := CloneOrUpdateRepo(name, repo, dryRun); err != nil {
			return fmt.Errorf("repo '%s' failed: %w", name, err)
		}
	}
	return nil
}
