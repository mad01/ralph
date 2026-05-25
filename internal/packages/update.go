package packages

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/hooks"
	"github.com/mad01/ralph/internal/progress"
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
	Verbose         bool
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
	Verbose         bool
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

	// go-install packages don't need target or working_dir resolution
	if pkg.Source == "go-install" {
		return resolved
	}

	if pkg.Source == "remote" || pkg.Source == "make" {
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

	keys := sortedPackageKeys(packages)
	prog := progress.New("Sync", len(keys))
	if opts.Verbose || opts.DryRun {
		prog = progress.NewQuiet()
	}

	for _, name := range keys {
		pkg := packages[name]
		if opts.SpecificPackage != "" && opts.SpecificPackage != name {
			continue
		}

		prog.TickWith(name)

		source := pkg.Source
		if source == "" {
			source = "local"
		}

		if !config.IsEnabled(pkg.Enable) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (disabled)\n", name, source)
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: fmt.Sprintf("disabled [%s]", source)})
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (host filter)\n", name, source)
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: fmt.Sprintf("host filter [%s]", source)})
			continue
		}

		if pkg.Source == "go-install" {
			fmt.Fprintf(w, "  Skipping package: %s [go-install] (nothing to sync)\n", name)
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: "go-install package (nothing to sync)"})
			continue
		}

		if pkg.Source == "local" || pkg.Source == "" {
			fmt.Fprintf(w, "  Skipping package: %s [local] (nothing to sync)\n", name)
			results = append(results, SyncResult{Name: name, Action: "skipped", Message: "local package"})
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := syncRemotePackage(w, name, resolved, opts)
		results = append(results, result)
	}
	prog.Done()

	return results
}

func syncRemotePackage(w io.Writer, name string, pkg config.Package, opts SyncOptions) SyncResult {
	target := pkg.Target

	if _, err := os.Stat(target); os.IsNotExist(err) {
		fmt.Fprintf(w, "  Package %s [remote]: cloning %s → %s\n", name, pkg.Repo, target)
		if err := gitClone(w, pkg.Repo, target, pkg.Branch, opts.DryRun, opts.Verbose); err != nil {
			return SyncResult{Name: name, Action: "error", Message: "clone failed", Err: err}
		}
		if opts.DryRun {
			return SyncResult{Name: name, Action: "cloned", Message: "[DRY RUN] would clone"}
		}
		return SyncResult{Name: name, Action: "cloned", Message: "cloned"}
	}

	fmt.Fprintf(w, "  Package %s [remote]: pulling latest...\n", name)
	if err := GitPull(w, target, opts.DryRun, opts.Verbose); err != nil {
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

	keys := sortedPackageKeys(packages)
	prog := progress.New("Packages", len(keys))
	if opts.Verbose || opts.DryRun {
		prog = progress.NewQuiet()
	}

	for _, name := range keys {
		pkg := packages[name]
		if opts.SpecificPackage != "" && opts.SpecificPackage != name {
			continue
		}

		prog.TickWith(name)

		source := pkg.Source
		if source == "" {
			source = "local"
		}

		if !config.IsEnabled(pkg.Enable) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (disabled)\n", name, source)
			results = append(results, BuildResult{Name: name, Action: "skipped", Message: fmt.Sprintf("disabled [%s]", source)})
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (host filter)\n", name, source)
			results = append(results, BuildResult{Name: name, Action: "skipped", Message: fmt.Sprintf("host filter [%s]", source)})
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := BuildPackage(w, name, resolved, opts)
		results = append(results, result)
	}
	prog.Done()

	return results
}

// BuildPackage detects changes and rebuilds a single package. The package
// paths should already be resolved via ResolvePackagePaths before calling.
func BuildPackage(w io.Writer, name string, pkg config.Package, opts BuildOptions) BuildResult {
	stateKey := "pkg:" + name
	source := pkg.Source
	if source == "" {
		source = "local"
	}

	// Handle go-install packages separately
	if pkg.Source == "go-install" {
		return buildGoInstallPackage(w, name, pkg, stateKey, opts)
	}

	workDir := pkg.WorkingDir

	// Apply make source defaults: if source=make and build/install are empty, use conventional defaults
	build := pkg.Build
	install := pkg.Install
	if pkg.Source == "make" {
		if len(build) == 0 {
			build = []string{"make build"}
		}
		if len(install) == 0 {
			install = []string{"make install"}
		}
	}

	// Check working dir exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		if pkg.Source == "remote" || pkg.Source == "make" {
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

	if err := runCommands(w, build, workDir, "build", pkg.Timeout, opts.DryRun, opts.Verbose); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "build failed", Err: err}
	}
	if err := runCommands(w, install, workDir, "install", pkg.Timeout, opts.DryRun, opts.Verbose); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "install failed", Err: err}
	}

	savePackageState(stateKey, workDir)
	return BuildResult{Name: name, Action: "built", Message: "rebuilt"}
}

func buildGoInstallPackage(w io.Writer, name string, pkg config.Package, stateKey string, opts BuildOptions) BuildResult {
	source := "go-install"

	if opts.DryRun {
		gobin := ""
		if len(pkg.InstallPaths) > 0 {
			if expanded, err := config.ExpandPath(pkg.InstallPaths[0]); err == nil {
				gobin = filepath.Dir(expanded)
			}
		}
		cmdStr := fmt.Sprintf("GOBIN=%s go install %s@%s", gobin, pkg.Module, pkg.Version)
		fmt.Fprintf(w, "  Package %s [%s]: [DRY RUN] would run: %s\n", name, source, cmdStr)
		return BuildResult{Name: name, Action: "built", Message: "[DRY RUN] would go install"}
	}

	// Check if version has changed
	if !opts.Force {
		state, err := hooks.LoadBuildState()
		if err == nil {
			if record, exists := state.Builds[stateKey]; exists && record.Version == pkg.Version {
				fmt.Fprintf(w, "  Package %s [%s]: up to date (%s)\n", name, source, pkg.Version)
				return BuildResult{Name: name, Action: "up-to-date", Message: fmt.Sprintf("version %s already installed", pkg.Version)}
			}
		}
	}

	// Check that go is available
	if _, err := exec.LookPath("go"); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "go not found in PATH", Err: fmt.Errorf("go command not available: %w", err)}
	}

	// Determine GOBIN from first install_path
	gobin := ""
	if len(pkg.InstallPaths) > 0 {
		expanded, err := config.ExpandPath(pkg.InstallPaths[0])
		if err != nil {
			return BuildResult{Name: name, Action: "error", Message: "failed to expand install_path", Err: err}
		}
		gobin = filepath.Dir(expanded)

		// Ensure GOBIN directory exists
		if err := os.MkdirAll(gobin, 0755); err != nil {
			return BuildResult{Name: name, Action: "error", Message: "failed to create GOBIN directory", Err: err}
		}
	}

	cmdStr := fmt.Sprintf("GOBIN=%s go install %s@%s", gobin, pkg.Module, pkg.Version)
	fmt.Fprintf(w, "  Package %s [%s]: %s\n", name, source, cmdStr)

	// Set up timeout context
	timeout := time.Duration(pkg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 600 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var stderrBuf bytes.Buffer
	stderrW := io.Writer(os.Stderr)
	if !opts.Verbose {
		stderrW = &stderrBuf
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Stdout = w
	cmd.Stderr = stderrW

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return BuildResult{Name: name, Action: "error", Message: fmt.Sprintf("timed out after %ds", pkg.Timeout), Err: err}
		}
		if !opts.Verbose && stderrBuf.Len() > 0 {
			os.Stderr.Write(stderrBuf.Bytes())
		}
		return BuildResult{Name: name, Action: "error", Message: "go install failed", Err: err}
	}

	// Save state with version
	state, err := hooks.LoadBuildState()
	if err != nil {
		state = &hooks.BuildState{Builds: make(map[string]hooks.BuildRecord)}
	}
	state.Builds[stateKey] = hooks.BuildRecord{
		CompletedAt: time.Now(),
		Version:     pkg.Version,
	}
	_ = hooks.SaveBuildState(state)

	return BuildResult{Name: name, Action: "built", Message: fmt.Sprintf("installed %s@%s", pkg.Module, pkg.Version)}
}

func runCommands(w io.Writer, commands []string, workingDir string, label string, timeout int, dryRun, verbose bool) error {
	if len(commands) == 0 {
		return nil
	}

	// Set up timeout context
	timeoutDur := time.Duration(timeout) * time.Second
	if timeoutDur == 0 {
		timeoutDur = 600 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDur)
	defer cancel()

	var stderrBuf bytes.Buffer
	stderrW := io.Writer(os.Stderr)
	if !verbose {
		stderrW = &stderrBuf
	}

	for i, cmdStr := range commands {
		if dryRun {
			fmt.Fprintf(w, "    [DRY RUN] Would %s in '%s': %s\n", label, workingDir, cmdStr)
			continue
		}

		fmt.Fprintf(w, "    [%s %d/%d] %s\n", label, i+1, len(commands), cmdStr)

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		cmd.Stdout = w
		cmd.Stderr = stderrW
		if workingDir != "" {
			cmd.Dir = workingDir
		}

		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("%s timed out after %ds: %s", label, timeout, cmdStr)
			}
			if !verbose && stderrBuf.Len() > 0 {
				os.Stderr.Write(stderrBuf.Bytes())
			}
			return fmt.Errorf("%s command failed: %s: %w", label, cmdStr, err)
		}
	}

	return nil
}

// GitPull runs git pull in the given directory.
func GitPull(w io.Writer, dir string, dryRun, verbose bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git pull in %s\n", dir)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	var stderrBuf bytes.Buffer
	stderrW := io.Writer(os.Stderr)
	if !verbose {
		stderrW = &stderrBuf
	}

	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull timed out after 600s in %s", dir)
		}
		if !verbose && stderrBuf.Len() > 0 {
			os.Stderr.Write(stderrBuf.Bytes())
		}
		return err
	}
	return nil
}

func gitClone(w io.Writer, url, target, branch string, dryRun, verbose bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git clone %s %s\n", url, target)
		return nil
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	var stderrBuf bytes.Buffer
	stderrW := io.Writer(os.Stderr)
	if !verbose {
		stderrW = &stderrBuf
	}

	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, url, target)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after 600s: %s", url)
		}
		if !verbose && stderrBuf.Len() > 0 {
			os.Stderr.Write(stderrBuf.Bytes())
		}
		return err
	}
	return nil
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
