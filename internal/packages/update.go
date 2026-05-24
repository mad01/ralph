package packages

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
)

func sortedPackageKeys(packages map[string]config.Package) []string {
	keys := make([]string, 0, len(packages))
	for name := range packages {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

// SyncOptions holds options for the sync command.
type SyncOptions struct {
	DryRun          bool
	SpecificPackage string
}

// SyncResult holds the result of syncing a single package.
type SyncResult struct {
	Name    string
	Action  string // "cloned", "pulled", "skipped", "up-to-date", "error"
	Message string
	Err     error
}

// BuildOptions holds options for building packages.
type BuildOptions struct {
	DryRun          bool
	Force           bool
	SpecificPackage string
}

// BuildResult holds the result of building a single package.
type BuildResult struct {
	Name    string
	Action  string // "built", "up-to-date", "skipped", "error"
	Message string
	Err     error
}

// ResolvePackagePaths resolves default target and working_dir for a package.
func ResolvePackagePaths(name string, pkg config.Package, packagesDir string) config.Package {
	if packagesDir == "" {
		packagesDir = config.DefaultPackagesDir
	}

	resolved := pkg

	if pkg.Source == "remote" {
		if resolved.Target == "" {
			expandedDir, err := config.ExpandPath(packagesDir)
			if err == nil {
				resolved.Target = filepath.Join(expandedDir, name)
			} else {
				resolved.Target = filepath.Join(packagesDir, name)
			}
		} else {
			expandedTarget, err := config.ExpandPath(resolved.Target)
			if err == nil {
				resolved.Target = expandedTarget
			}
		}
		if resolved.WorkingDir == "" {
			resolved.WorkingDir = resolved.Target
		} else {
			expandedDir, err := config.ExpandPath(resolved.WorkingDir)
			if err == nil {
				resolved.WorkingDir = expandedDir
			}
		}
	} else {
		// Local package
		if resolved.WorkingDir != "" {
			expandedDir, err := config.ExpandPath(resolved.WorkingDir)
			if err == nil {
				resolved.WorkingDir = expandedDir
			}
		}
	}

	return resolved
}

// SyncPackages clones or pulls remote packages. Local packages are skipped.
func SyncPackages(w io.Writer, packages map[string]config.Package, packagesDir string, currentHost string, opts SyncOptions) []SyncResult {
	var results []SyncResult

	if len(packages) == 0 {
		return results
	}

	// Sorted keys for deterministic order
	keys := sortedPackageKeys(packages)
	total := len(keys)

	for i, name := range keys {
		pkg := packages[name]
		if opts.SpecificPackage != "" && opts.SpecificPackage != name {
			continue
		}

		source := pkg.Source
		if source == "" {
			source = "local"
		}

		fmt.Fprintf(os.Stdout, "  [%d/%d] %s\n", i+1, total, name)

		if !config.IsEnabled(pkg.Enable) {
			fmt.Fprintf(w, "    skipped (disabled)\n")
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: fmt.Sprintf("disabled [%s]", source)})
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "    skipped (host filter)\n")
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: fmt.Sprintf("host filter [%s]", source)})
			continue
		}

		if pkg.Source == "local" || pkg.Source == "" {
			fmt.Fprintf(w, "    skipped (local, nothing to sync)\n")
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: "local package"})
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := syncRemotePackage(w, name, resolved, opts)
		results = append(results, result)
	}

	return results
}

func syncRemotePackage(w io.Writer, name string, pkg config.Package, opts SyncOptions) SyncResult {
	target := pkg.Target

	if _, err := os.Stat(target); os.IsNotExist(err) {
		fmt.Fprintf(w, "  Package %s [remote]: cloning %s → %s\n", name, pkg.Repo, target)
		if err := gitClone(w, pkg.Repo, target, pkg.Branch, opts.DryRun); err != nil {
			return SyncResult{Name: name, Action: "error", Message: "clone failed", Err: err}
		}
		if opts.DryRun {
			return SyncResult{Name: name, Action: "cloned", Message: "[DRY RUN] would clone"}
		}
		return SyncResult{Name: name, Action: "cloned", Message: "cloned"}
	}

	fmt.Fprintf(w, "  Package %s [remote]: pulling latest...\n", name)
	if err := GitPull(w, target, opts.DryRun); err != nil {
		return SyncResult{Name: name, Action: "error", Message: "pull failed", Err: err}
	}

	if opts.DryRun {
		return SyncResult{Name: name, Action: "pulled", Message: "[DRY RUN] would pull"}
	}
	return SyncResult{Name: name, Action: "pulled", Message: "pulled"}
}

// BuildPackages detects changes and rebuilds packages as needed.
func BuildPackages(w io.Writer, packages map[string]config.Package, packagesDir string, currentHost string, opts BuildOptions) []BuildResult {
	var results []BuildResult

	if len(packages) == 0 {
		return results
	}

	// Sorted keys for deterministic order
	keys := sortedPackageKeys(packages)
	total := len(keys)

	for i, name := range keys {
		pkg := packages[name]
		if opts.SpecificPackage != "" && opts.SpecificPackage != name {
			continue
		}

		source := pkg.Source
		if source == "" {
			source = "local"
		}

		fmt.Fprintf(os.Stdout, "  [%d/%d] %s\n", i+1, total, name)

		if !config.IsEnabled(pkg.Enable) {
			fmt.Fprintf(w, "    skipped (disabled)\n")
			results = append(results, BuildResult{Name: name, Action: "skipped", Message: fmt.Sprintf("disabled [%s]", source)})
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "    skipped (host filter)\n")
			results = append(results, BuildResult{Name: name, Action: "skipped", Message: fmt.Sprintf("host filter [%s]", source)})
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := buildPackage(w, name, resolved, opts)
		results = append(results, result)
	}

	return results
}

func buildPackage(w io.Writer, name string, pkg config.Package, opts BuildOptions) BuildResult {
	stateKey := "pkg:" + name
	workDir := pkg.WorkingDir
	source := pkg.Source
	if source == "" {
		source = "local"
	}

	// Check working dir exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		if pkg.Source == "remote" {
			return BuildResult{Name: name, Action: "skipped", Message: "not cloned (run 'ralph sync' first)"}
		}
		return BuildResult{Name: name, Action: "error", Message: fmt.Sprintf("working_dir '%s' does not exist", workDir), Err: err}
	}

	if opts.DryRun {
		fmt.Fprintf(w, "  Package %s [%s]: [DRY RUN] would check for changes and rebuild\n", name, source)
		return BuildResult{Name: name, Action: "built", Message: "[DRY RUN] would check and rebuild if changed"}
	}

	currentHash := hooks.GetGitHash(workDir)
	hasUncommitted := hooks.HasGitChanges(workDir)

	needsBuild := opts.Force
	if !needsBuild {
		state, err := hooks.LoadBuildState()
		if err == nil {
			if record, exists := state.Builds[stateKey]; exists {
				if currentHash != "" && record.GitHash != "" && currentHash != record.GitHash {
					fmt.Fprintf(w, "  Package %s [%s]: git changes detected (%s → %s)\n", name, source, short(record.GitHash), short(currentHash))
					needsBuild = true
				} else if hasUncommitted {
					fmt.Fprintf(w, "  Package %s [%s]: uncommitted changes detected\n", name, source)
					needsBuild = true
				}
			} else {
				needsBuild = true
			}
		} else {
			needsBuild = true
		}
	}

	if !needsBuild {
		fmt.Fprintf(w, "  Package %s [%s]: up to date\n", name, source)
		return BuildResult{Name: name, Action: "up-to-date", Message: "no changes detected"}
	}

	if opts.Force {
		fmt.Fprintf(w, "  Package %s [%s]: force rebuild\n", name, source)
	}

	if err := runCommands(w, pkg.Build, workDir, "build", opts.DryRun); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "build failed", Err: err}
	}
	if err := runCommands(w, pkg.Install, workDir, "install", opts.DryRun); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "install failed", Err: err}
	}

	savePackageState(stateKey, workDir)
	return BuildResult{Name: name, Action: "built", Message: "rebuilt"}
}

func runCommands(w io.Writer, commands []string, workingDir string, label string, dryRun bool) error {
	if len(commands) == 0 {
		return nil
	}

	for i, cmdStr := range commands {
		if dryRun {
			fmt.Fprintf(w, "    [DRY RUN] Would %s in '%s': %s\n", label, workingDir, cmdStr)
			continue
		}

		fmt.Fprintf(w, "    [%s %d/%d] %s\n", label, i+1, len(commands), cmdStr)

		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Stdout = w
		cmd.Stderr = os.Stderr
		if workingDir != "" {
			cmd.Dir = workingDir
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s command failed: %s: %w", label, cmdStr, err)
		}
	}

	return nil
}

// GitPull runs git pull in the given directory.
func GitPull(w io.Writer, dir string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git pull in %s\n", dir)
		return nil
	}

	cmd := exec.Command("git", "pull")
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitClone(w io.Writer, url, target, branch string, dryRun bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git clone %s %s\n", url, target)
		return nil
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, url, target)

	cmd := exec.Command("git", args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func savePackageState(stateKey, workDir string) {
	state, err := hooks.LoadBuildState()
	if err != nil {
		return
	}
	record := hooks.BuildRecord{
		CompletedAt: time.Now(),
	}
	if hash := hooks.GetGitHash(workDir); hash != "" {
		record.GitHash = hash
	}
	state.Builds[stateKey] = record
	_ = hooks.SaveBuildState(state)
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
