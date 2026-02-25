package packages

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
)

// UpdateOptions holds options for the update command.
type UpdateOptions struct {
	DryRun          bool
	Force           bool
	SpecificPackage string
}

// UpdateResult holds the result of updating a single package.
type UpdateResult struct {
	Name    string
	Action  string // "updated", "up-to-date", "cloned", "skipped", "error"
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

// UpdatePackages updates all applicable packages.
func UpdatePackages(w io.Writer, packages map[string]config.Package, packagesDir string, currentHost string, opts UpdateOptions) []UpdateResult {
	var results []UpdateResult

	if len(packages) == 0 {
		return results
	}

	for name, pkg := range packages {
		// Filter by specific package if requested
		if opts.SpecificPackage != "" && opts.SpecificPackage != name {
			continue
		}

		// Check enable
		if !config.IsEnabled(pkg.Enable) {
			fmt.Fprintf(w, "  Skipping package: %s (disabled)\n", name)
			results = append(results, UpdateResult{Name: name, Action: "skipped", Message: "disabled"})
			continue
		}

		// Check host filter
		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping package: %s (host filter)\n", name)
			results = append(results, UpdateResult{Name: name, Action: "skipped", Message: "host filter"})
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := updatePackage(w, name, resolved, opts)
		results = append(results, result)
	}

	return results
}

// updatePackage performs the update for a single package.
func updatePackage(w io.Writer, name string, pkg config.Package, opts UpdateOptions) UpdateResult {
	stateKey := "pkg:" + name

	switch pkg.Source {
	case "remote":
		return updateRemotePackage(w, name, pkg, stateKey, opts)
	case "local":
		return updateLocalPackage(w, name, pkg, stateKey, opts)
	default:
		return UpdateResult{Name: name, Action: "error", Err: fmt.Errorf("unknown source type: %s", pkg.Source)}
	}
}

func updateRemotePackage(w io.Writer, name string, pkg config.Package, stateKey string, opts UpdateOptions) UpdateResult {
	target := pkg.Target

	// Check if target exists
	if _, err := os.Stat(target); os.IsNotExist(err) {
		// Clone
		fmt.Fprintf(w, "  Package %s: cloning %s → %s\n", name, pkg.Repo, target)
		if err := gitClone(w, pkg.Repo, target, pkg.Branch, opts.DryRun); err != nil {
			return UpdateResult{Name: name, Action: "error", Message: "clone failed", Err: err}
		}
		if opts.DryRun {
			return UpdateResult{Name: name, Action: "cloned", Message: "[DRY RUN] would clone and build"}
		}

		// Build + install after clone
		if err := runCommands(w, pkg.Build, pkg.WorkingDir, "build", opts.DryRun); err != nil {
			return UpdateResult{Name: name, Action: "error", Message: "build failed after clone", Err: err}
		}
		if err := runCommands(w, pkg.Install, pkg.WorkingDir, "install", opts.DryRun); err != nil {
			return UpdateResult{Name: name, Action: "error", Message: "install failed after clone", Err: err}
		}

		// Save state
		savePackageState(stateKey, pkg.WorkingDir)
		return UpdateResult{Name: name, Action: "cloned", Message: "cloned and built"}
	}

	// Get hash before pull
	hashBefore := hooks.GetGitHash(target)

	// Pull
	fmt.Fprintf(w, "  Package %s: pulling latest...\n", name)
	if err := GitPull(w, target, opts.DryRun); err != nil {
		return UpdateResult{Name: name, Action: "error", Message: "pull failed", Err: err}
	}

	if opts.DryRun {
		return UpdateResult{Name: name, Action: "updated", Message: "[DRY RUN] would pull and rebuild if changed"}
	}

	// Get hash after pull
	hashAfter := hooks.GetGitHash(target)

	// Check if rebuild needed
	needsBuild := opts.Force || hashBefore != hashAfter
	if !needsBuild {
		// Also check state
		state, err := hooks.LoadBuildState()
		if err == nil {
			if _, exists := state.Builds[stateKey]; !exists {
				needsBuild = true
			}
		}
	}

	if !needsBuild {
		fmt.Fprintf(w, "  Package %s: up to date (no changes)\n", name)
		return UpdateResult{Name: name, Action: "up-to-date", Message: "no changes detected"}
	}

	if hashBefore != hashAfter {
		fmt.Fprintf(w, "  Package %s: changes detected (%s → %s)\n", name, short(hashBefore), short(hashAfter))
	} else {
		fmt.Fprintf(w, "  Package %s: force rebuild\n", name)
	}

	// Build + install
	if err := runCommands(w, pkg.Build, pkg.WorkingDir, "build", opts.DryRun); err != nil {
		return UpdateResult{Name: name, Action: "error", Message: "build failed", Err: err}
	}
	if err := runCommands(w, pkg.Install, pkg.WorkingDir, "install", opts.DryRun); err != nil {
		return UpdateResult{Name: name, Action: "error", Message: "install failed", Err: err}
	}

	savePackageState(stateKey, pkg.WorkingDir)
	return UpdateResult{Name: name, Action: "updated", Message: "rebuilt"}
}

func updateLocalPackage(w io.Writer, name string, pkg config.Package, stateKey string, opts UpdateOptions) UpdateResult {
	workDir := pkg.WorkingDir

	// Check working dir exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return UpdateResult{Name: name, Action: "error", Message: fmt.Sprintf("working_dir '%s' does not exist", workDir), Err: err}
	}

	if opts.DryRun {
		fmt.Fprintf(w, "  Package %s: [DRY RUN] would check for changes and rebuild\n", name)
		return UpdateResult{Name: name, Action: "updated", Message: "[DRY RUN] would check and rebuild if changed"}
	}

	currentHash := hooks.GetGitHash(workDir)
	hasUncommitted := hooks.HasGitChanges(workDir)

	// Check state
	needsBuild := opts.Force
	if !needsBuild {
		state, err := hooks.LoadBuildState()
		if err == nil {
			if record, exists := state.Builds[stateKey]; exists {
				if currentHash != "" && record.GitHash != "" && currentHash != record.GitHash {
					fmt.Fprintf(w, "  Package %s: git changes detected (%s → %s)\n", name, short(record.GitHash), short(currentHash))
					needsBuild = true
				} else if hasUncommitted {
					fmt.Fprintf(w, "  Package %s: uncommitted changes detected\n", name)
					needsBuild = true
				}
			} else {
				// No prior state
				needsBuild = true
			}
		} else {
			needsBuild = true
		}
	}

	if !needsBuild {
		fmt.Fprintf(w, "  Package %s: up to date\n", name)
		return UpdateResult{Name: name, Action: "up-to-date", Message: "no changes detected"}
	}

	if opts.Force {
		fmt.Fprintf(w, "  Package %s: force rebuild\n", name)
	}

	// Build + install
	if err := runCommands(w, pkg.Build, workDir, "build", opts.DryRun); err != nil {
		return UpdateResult{Name: name, Action: "error", Message: "build failed", Err: err}
	}
	if err := runCommands(w, pkg.Install, workDir, "install", opts.DryRun); err != nil {
		return UpdateResult{Name: name, Action: "error", Message: "install failed", Err: err}
	}

	savePackageState(stateKey, workDir)
	return UpdateResult{Name: name, Action: "updated", Message: "rebuilt"}
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
