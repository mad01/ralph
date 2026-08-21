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
	"strings"
	"time"

	"github.com/mad01/ralph/internal/binversion"
	"github.com/mad01/ralph/internal/buildinfo"
	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/gitutil"
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
	Name             string
	Action           string // "built", "up-to-date", "skipped", "error"
	Message          string
	Err              error
	ServiceRestarted bool // true when a package [service] restart command ran after the binary changed
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
func SyncPackages(
	ctx context.Context,
	w io.Writer,
	packages map[string]config.Package,
	packagesDir string,
	currentHost string,
	opts SyncOptions,
) []SyncResult {
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
			results = append(
				results,
				SyncResult{
					Name:    name,
					Action:  "skipped",
					Message: fmt.Sprintf("disabled [%s]", source),
				},
			)
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (host filter)\n", name, source)
			results = append(
				results,
				SyncResult{
					Name:    name,
					Action:  "skipped",
					Message: fmt.Sprintf("host filter [%s]", source),
				},
			)
			continue
		}

		if pkg.Source == "go-install" {
			fmt.Fprintf(w, "  Skipping package: %s [go-install] (nothing to sync)\n", name)
			results = append(
				results,
				SyncResult{
					Name:    name,
					Action:  "skipped",
					Message: "go-install package (nothing to sync)",
				},
			)
			continue
		}

		if pkg.Source == "local" || pkg.Source == "" {
			fmt.Fprintf(w, "  Skipping package: %s [local] (nothing to sync)\n", name)
			results = append(
				results,
				SyncResult{Name: name, Action: "skipped", Message: "local package"},
			)
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := syncRemotePackage(ctx, w, name, resolved, opts)
		results = append(results, result)
	}
	prog.Done()

	return results
}

func syncRemotePackage(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts SyncOptions,
) SyncResult {
	target := pkg.Target

	if _, err := os.Stat(target); os.IsNotExist(err) {
		fmt.Fprintf(w, "  Package %s [remote]: cloning %s → %s\n", name, pkg.Repo, target)
		if err := gitClone(ctx, w, pkg.Repo, target, pkg.Branch, opts.DryRun, opts.Verbose); err != nil {
			return SyncResult{Name: name, Action: "error", Message: "clone failed", Err: err}
		}
		if opts.DryRun {
			return SyncResult{Name: name, Action: "cloned", Message: "[DRY RUN] would clone"}
		}
		return SyncResult{Name: name, Action: "cloned", Message: "cloned"}
	}

	fmt.Fprintf(w, "  Package %s [remote]: pulling latest...\n", name)
	if err := GitPull(ctx, w, target, opts.DryRun, opts.Verbose); err != nil {
		return SyncResult{Name: name, Action: "error", Message: "pull failed", Err: err}
	}

	if opts.DryRun {
		return SyncResult{Name: name, Action: "pulled", Message: "[DRY RUN] would pull"}
	}
	return SyncResult{Name: name, Action: "pulled", Message: "pulled"}
}

// BuildPackages detects changes and rebuilds packages as needed.
func BuildPackages(
	ctx context.Context,
	w io.Writer,
	packages map[string]config.Package,
	packagesDir string,
	currentHost string,
	opts BuildOptions,
) []BuildResult {
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
			results = append(
				results,
				BuildResult{
					Name:    name,
					Action:  "skipped",
					Message: fmt.Sprintf("disabled [%s]", source),
				},
			)
			continue
		}

		if !config.ShouldApplyForHost(pkg.Hosts, currentHost) {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (host filter)\n", name, source)
			results = append(
				results,
				BuildResult{
					Name:    name,
					Action:  "skipped",
					Message: fmt.Sprintf("host filter [%s]", source),
				},
			)
			continue
		}

		resolved := ResolvePackagePaths(name, pkg, packagesDir)
		result := BuildPackage(ctx, w, name, resolved, opts)
		results = append(results, result)
	}
	prog.Done()

	return results
}

// BuildPackage detects changes and rebuilds a single package. The package
// paths should already be resolved via ResolvePackagePaths before calling.
// firstMissingInstallPath returns the first declared install_path that does
// not exist on disk (after ~ expansion). Cleanup or an accidental delete can
// remove an installed binary while the package source is unchanged; treating a
// missing install_path as "needs build" lets a normal `ralph up` reinstall it
// instead of requiring --reset-builds.
func firstMissingInstallPath(pkg config.Package) (string, bool) {
	for _, ip := range pkg.InstallPaths {
		expanded, err := config.ExpandPath(ip)
		if err != nil {
			continue
		}
		if _, err := os.Stat(expanded); os.IsNotExist(err) {
			return expanded, true
		}
	}
	return "", false
}

// InstallationCheck describes whether an installed package still agrees with
// the source and the state recorded after the last successful install.
type InstallationCheck struct {
	BuildInfo     buildinfo.Info
	VersionProbed bool
	Reason        string
}

// ReportedGitRevision returns the SHA-shaped revision reported by a binary.
// The explicit commit wins; legacy binaries may report a 7-40 character short
// SHA in version. Full SHA-256 object IDs are also accepted. Release versions
// such as v1.2.3 do not produce a verdict.
func ReportedGitRevision(info buildinfo.Info) string {
	if isGitRevision(info.Commit) {
		return strings.ToLower(info.Commit)
	}
	if isGitRevision(info.Version) {
		return strings.ToLower(info.Version)
	}
	return ""
}

func isGitRevision(value string) bool {
	if len(value) < 7 || (len(value) > 40 && len(value) != 64) {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// GitRevisionMatchesHead compares a full or abbreviated SHA with a full HEAD
// SHA. Both inputs must be SHA-shaped; a short revision matches a HEAD prefix.
func GitRevisionMatchesHead(revision, head string) bool {
	if !isGitRevision(revision) || !isGitRevision(head) || len(revision) > len(head) {
		return false
	}
	return strings.HasPrefix(strings.ToLower(head), strings.ToLower(revision))
}

// CheckInstalledPackage applies the same installed-artifact freshness policy
// used by ralph up, list, and doctor. Version probing is opt-in because some
// binaries interpret `version -o json` as a mutating command. When enabled, a
// usable reported revision is compared with the repository-wide HEAD. The
// content hash remains an integrity guard, including when versions match.
func CheckInstalledPackage(
	pkg config.Package,
	workDir string,
	record buildstate.BuildRecord,
) InstallationCheck {
	var check InstallationCheck
	if len(pkg.InstallPaths) == 0 {
		return check
	}

	expanded := make([]string, 0, len(pkg.InstallPaths))
	for _, path := range pkg.InstallPaths {
		resolved, err := config.ExpandPath(path)
		if err != nil {
			check.Reason = fmt.Sprintf("cannot expand install_path %q", path)
			return check
		}
		if _, err := os.Stat(resolved); err != nil {
			if os.IsNotExist(err) {
				check.Reason = fmt.Sprintf("install_path missing (%s)", resolved)
			} else {
				check.Reason = fmt.Sprintf("cannot inspect install_path %s", resolved)
			}
			return check
		}
		expanded = append(expanded, resolved)
	}

	if pkg.VersionCheck {
		check.BuildInfo, check.VersionProbed = binversion.Probe(expanded[0])
		if !check.VersionProbed {
			check.Reason = "installed binary does not support version -o json"
			return check
		}
		revision := ReportedGitRevision(check.BuildInfo)
		if revision == "" {
			check.Reason = "installed binary did not report a SHA-shaped commit or version"
			return check
		}
		head := gitutil.GetGitHash(workDir)
		if head == "" {
			check.Reason = "cannot determine source repository HEAD"
			return check
		}
		if !GitRevisionMatchesHead(revision, head) {
			check.Reason = fmt.Sprintf(
				"installed revision %s does not match source HEAD %s",
				short(revision),
				short(head),
			)
			return check
		}
	}

	if record.InstallHash == "" {
		check.Reason = "no recorded install hash"
		return check
	}
	currentHash, err := buildstate.ComputeInstallHash(expanded)
	if err != nil {
		check.Reason = "cannot verify installed content"
		return check
	}
	if currentHash != record.InstallHash {
		check.Reason = "installed content differs from the last successful install"
	}
	return check
}

func validateInstalledVersion(pkg config.Package, workDir string) error {
	if !pkg.VersionCheck {
		return nil
	}
	if len(pkg.InstallPaths) == 0 {
		return fmt.Errorf("version_check requires install_paths")
	}
	path, err := config.ExpandPath(pkg.InstallPaths[0])
	if err != nil {
		return fmt.Errorf("expand version_check install_path: %w", err)
	}
	info, ok := binversion.Probe(path)
	if !ok {
		return fmt.Errorf("installed binary does not support version -o json")
	}
	revision := ReportedGitRevision(info)
	if revision == "" {
		return fmt.Errorf("installed binary did not report a SHA-shaped commit or version")
	}
	head := gitutil.GetGitHash(workDir)
	if head == "" {
		return fmt.Errorf("cannot determine source repository HEAD")
	}
	if !GitRevisionMatchesHead(revision, head) {
		return fmt.Errorf(
			"installed revision %s does not match source HEAD %s",
			short(revision),
			short(head),
		)
	}
	return nil
}

func BuildPackage(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts BuildOptions,
) BuildResult {
	stateKey := "pkg:" + name
	source := pkg.Source
	if source == "" {
		source = "local"
	}

	// Handle go-install packages separately
	if pkg.Source == "go-install" {
		return buildGoInstallPackage(ctx, w, name, pkg, stateKey, opts)
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
			return BuildResult{
				Name:    name,
				Action:  "skipped",
				Message: "not cloned (run 'ralph sync' first)",
			}
		}
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: fmt.Sprintf("working_dir '%s' does not exist", workDir),
			Err:     err,
		}
	}

	if opts.DryRun {
		fmt.Fprintf(
			w,
			"  Package %s [%s]: [DRY RUN] would check for changes and rebuild\n",
			name,
			source,
		)
		return BuildResult{
			Name:    name,
			Action:  "built",
			Message: "[DRY RUN] would check and rebuild if changed",
		}
	}

	// Tree hash of working_dir's subtree (not the repo-wide commit), so commits
	// elsewhere in the repo don't force a rebuild; ignored build output is
	// excluded from the uncommitted-changes check.
	currentHash := gitutil.GetTreeHash(workDir)
	hasUncommitted := gitutil.HasGitChangesInPath(workDir)

	needsBuild := opts.Force
	var recordedBuild buildstate.BuildRecord
	if !needsBuild {
		state, err := buildstate.LoadBuildState()
		if err == nil {
			if record, exists := state.Builds[stateKey]; exists {
				recordedBuild = record
				switch {
				case currentHash != "" && record.GitHash == "":
					// A record saved while git was failing carries no source
					// hash and could never match, so without this rebuild the
					// package would stay frozen on its stale binary through
					// every future source change. Rebuilding re-records a real
					// hash and change detection resumes.
					fmt.Fprintf(
						w,
						"  Package %s [%s]: no recorded source hash, rebuilding to repair state\n",
						name,
						source,
					)
					needsBuild = true
				case currentHash != "" && currentHash != record.GitHash:
					fmt.Fprintf(
						w,
						"  Package %s [%s]: git changes detected (%s → %s)\n",
						name,
						source,
						short(record.GitHash),
						short(currentHash),
					)
					needsBuild = true
				case hasUncommitted:
					fmt.Fprintf(
						w,
						"  Package %s [%s]: uncommitted changes detected\n",
						name,
						source,
					)
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
		if check := CheckInstalledPackage(pkg, workDir, recordedBuild); check.Reason != "" {
			fmt.Fprintf(
				w,
				"  Package %s [%s]: %s, rebuilding\n",
				name,
				source,
				check.Reason,
			)
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

	prevInstallHash := loadInstallHash(stateKey)

	if err := runCommands(ctx, w, build, workDir, "build", pkg.Timeout, opts.DryRun, opts.Verbose); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "build failed", Err: err}
	}
	if err := runCommands(ctx, w, install, workDir, "install", pkg.Timeout, opts.DryRun, opts.Verbose); err != nil {
		return BuildResult{Name: name, Action: "error", Message: "install failed", Err: err}
	}
	if err := validateInstalledVersion(pkg, workDir); err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "installed version validation failed",
			Err:     err,
		}
	}

	newInstallHash, err := validatedInstallHash(pkg)
	if err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "installed artifact validation failed",
			Err:     err,
		}
	}
	savePackageState(w, stateKey, workDir, newInstallHash)
	result := BuildResult{Name: name, Action: "built", Message: "rebuilt"}
	if maybeRestartService(ctx, w, name, pkg, prevInstallHash, newInstallHash, opts) {
		result.ServiceRestarted = true
		result.Message = "rebuilt; service restarted"
	}
	return result
}

func buildGoInstallPackage(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	stateKey string,
	opts BuildOptions,
) BuildResult {
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

	// Check if version or the installed artifact has changed.
	if !opts.Force {
		state, err := buildstate.LoadBuildState()
		if err == nil {
			if record, exists := state.Builds[stateKey]; exists && record.Version == pkg.Version {
				if check := CheckInstalledPackage(pkg, "", record); check.Reason != "" {
					fmt.Fprintf(
						w,
						"  Package %s [%s]: %s, reinstalling\n",
						name,
						source,
						check.Reason,
					)
				} else {
					fmt.Fprintf(w, "  Package %s [%s]: up to date (%s)\n", name, source, pkg.Version)
					return BuildResult{Name: name, Action: "up-to-date", Message: fmt.Sprintf("version %s already installed", pkg.Version)}
				}
			}
		}
	}

	// Check that go is available
	if _, err := exec.LookPath("go"); err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "go not found in PATH",
			Err:     fmt.Errorf("go command not available: %w", err),
		}
	}

	// Determine GOBIN from first install_path
	gobin := ""
	if len(pkg.InstallPaths) > 0 {
		expanded, err := config.ExpandPath(pkg.InstallPaths[0])
		if err != nil {
			return BuildResult{
				Name:    name,
				Action:  "error",
				Message: "failed to expand install_path",
				Err:     err,
			}
		}
		gobin = filepath.Dir(expanded)

		// Ensure GOBIN directory exists
		if err := os.MkdirAll(gobin, 0o755); err != nil {
			return BuildResult{
				Name:    name,
				Action:  "error",
				Message: "failed to create GOBIN directory",
				Err:     err,
			}
		}
	}

	cmdStr := fmt.Sprintf("GOBIN=%s go install %s@%s", gobin, pkg.Module, pkg.Version)
	fmt.Fprintf(w, "  Package %s [%s]: %s\n", name, source, cmdStr)

	// Set up timeout context
	timeout := time.Duration(pkg.Timeout) * time.Second
	if timeout == 0 {
		timeout = config.DefaultExecTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
			return BuildResult{
				Name:    name,
				Action:  "error",
				Message: fmt.Sprintf("timed out after %ds", pkg.Timeout),
				Err:     err,
			}
		}
		if !opts.Verbose && stderrBuf.Len() > 0 {
			_, _ = os.Stderr.Write(stderrBuf.Bytes())
		}
		return BuildResult{Name: name, Action: "error", Message: "go install failed", Err: err}
	}

	// Save state with version
	prevInstallHash := loadInstallHash(stateKey)
	newInstallHash, err := validatedInstallHash(pkg)
	if err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "installed artifact validation failed",
			Err:     err,
		}
	}
	state, err := buildstate.LoadBuildState()
	if err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "failed to load build state",
			Err:     err,
		}
	}
	if state.Builds == nil {
		state.Builds = make(map[string]buildstate.BuildRecord)
	}
	state.Builds[stateKey] = buildstate.BuildRecord{
		CompletedAt: time.Now(),
		Version:     pkg.Version,
		InstallHash: newInstallHash,
	}
	if err := buildstate.SaveBuildState(state); err != nil {
		return BuildResult{
			Name:    name,
			Action:  "error",
			Message: "failed to save build state",
			Err:     err,
		}
	}

	result := BuildResult{
		Name:    name,
		Action:  "built",
		Message: fmt.Sprintf("installed %s@%s", pkg.Module, pkg.Version),
	}
	if maybeRestartService(ctx, w, name, pkg, prevInstallHash, newInstallHash, opts) {
		result.ServiceRestarted = true
		result.Message += "; service restarted"
	}
	return result
}

func runCommands(
	ctx context.Context,
	w io.Writer,
	commands []string,
	workingDir string,
	label string,
	timeout int,
	dryRun, verbose bool,
) error {
	if len(commands) == 0 {
		return nil
	}

	// Set up timeout context
	timeoutDur := time.Duration(timeout) * time.Second
	if timeoutDur == 0 {
		timeoutDur = config.DefaultExecTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeoutDur)
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
				_, _ = os.Stderr.Write(stderrBuf.Bytes())
			}
			return fmt.Errorf("%s command failed: %s: %w", label, cmdStr, err)
		}
	}

	return nil
}

// GitPull runs git pull in the given directory.
// If the current branch has no upstream tracking, it falls back to
// pulling from origin with the current branch name.
func GitPull(ctx context.Context, w io.Writer, dir string, dryRun, verbose bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git pull in %s\n", dir)
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()

	// Always capture stderr to a buffer so the no-tracking fallback below can
	// inspect it; in verbose mode also stream it to os.Stderr live.
	var stderrBuf bytes.Buffer
	stderrW := io.Writer(&stderrBuf)
	if verbose {
		stderrW = io.MultiWriter(os.Stderr, &stderrBuf)
	}

	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull timed out after 600s in %s", dir)
		}
		// If pull failed due to no tracking info, try pulling current branch from origin
		if stderrBuf.Len() > 0 && strings.Contains(stderrBuf.String(), "no tracking information") {
			branch := getCurrentBranch(dir)
			if branch != "" {
				stderrBuf.Reset()
				fmt.Fprintf(w, "    No tracking info, pulling origin/%s...\n", branch)
				retryCmd := exec.CommandContext(ctx, "git", "pull", "origin", branch)
				retryCmd.Dir = dir
				retryCmd.Stdout = w
				retryCmd.Stderr = stderrW
				if retryErr := retryCmd.Run(); retryErr == nil {
					return nil
				}
			}
		}
		if !verbose && stderrBuf.Len() > 0 {
			_, _ = os.Stderr.Write(stderrBuf.Bytes())
		}
		return err
	}
	return nil
}

// getCurrentBranch returns the current git branch name, or empty string on failure.
func getCurrentBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitClone(
	ctx context.Context,
	w io.Writer,
	url, target, branch string,
	dryRun, verbose bool,
) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git clone %s %s\n", url, target)
		return nil
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(target)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
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
	// "--" stops a "-"-prefixed URL from being parsed as a git option.
	args = append(args, "--", url, target)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after 600s: %s", url)
		}
		if !verbose && stderrBuf.Len() > 0 {
			_, _ = os.Stderr.Write(stderrBuf.Bytes())
		}
		return err
	}
	return nil
}

// savePackageState records a successful package build in the build-state file.
// A failure to load or save is logged (not fatal): the build itself succeeded,
// but a lost state write means the package looks stale and rebuilds on the next
// run. Silently swallowing the error left that as an unexplained perpetual
// rebuild, so surface it.
func savePackageState(w io.Writer, stateKey, workDir, installHash string) {
	state, err := buildstate.LoadBuildState()
	if err != nil {
		fmt.Fprintf(
			w,
			"  Warning: could not load build state to record %q (will rebuild next run): %v\n",
			stateKey,
			err,
		)
		return
	}
	record := buildstate.BuildRecord{
		CompletedAt: time.Now(),
		InstallHash: installHash,
	}
	if hash := gitutil.GetTreeHash(workDir); hash != "" {
		record.GitHash = hash
	} else {
		fmt.Fprintf(
			w,
			"  Warning: no source hash recorded for %q (git failed in %s); "+
				"source-change detection is off until a rebuild records one\n",
			stateKey,
			workDir,
		)
	}
	state.Builds[stateKey] = record
	if err := buildstate.SaveBuildState(state); err != nil {
		fmt.Fprintf(
			w,
			"  Warning: could not save build state for %q (will rebuild next run): %v\n",
			stateKey,
			err,
		)
	}
}

// loadInstallHash returns the install_paths content hash recorded for a package
// on its last build, or "" if none is stored.
func loadInstallHash(stateKey string) string {
	state, err := buildstate.LoadBuildState()
	if err != nil {
		return ""
	}
	if rec, ok := state.Builds[stateKey]; ok {
		return rec.InstallHash
	}
	return ""
}

// computeInstallHash hashes a package's install_paths contents (HOME-expanded).
// Returns "" if there are no paths or any path cannot be read — callers treat
// "" as "cannot determine" and skip the service restart rather than guessing.
func computeInstallHash(pkg config.Package) string {
	hash, err := validatedInstallHash(pkg)
	if err != nil {
		return ""
	}
	return hash
}

// validatedInstallHash hashes declared install paths and preserves errors so a
// successful command cannot bless missing or unreadable installed artifacts.
func validatedInstallHash(pkg config.Package) (string, error) {
	if len(pkg.InstallPaths) == 0 {
		return "", nil
	}
	expanded := make([]string, 0, len(pkg.InstallPaths))
	for _, p := range pkg.InstallPaths {
		e, err := config.ExpandPath(p)
		if err != nil {
			return "", fmt.Errorf("expand install_path %q: %w", p, err)
		}
		expanded = append(expanded, e)
	}
	h, err := buildstate.ComputeInstallHash(expanded)
	if err != nil {
		return "", err
	}
	if h == "" {
		return "", fmt.Errorf("declared install_paths produced an empty content hash")
	}
	return h, nil
}

// maybeRestartService runs a package's [service] restart command, but only when
// the installed binary's content changed since the last build (prevHash !=
// newHash, including the first build where prevHash is ""). It is best-effort:
// an inability to hash, or a failing restart command, is logged and never fails
// the build. Returns true only when the restart command actually ran and
// succeeded.
func maybeRestartService(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	prevHash, newHash string,
	opts BuildOptions,
) bool {
	if pkg.Service == nil || strings.TrimSpace(pkg.Service.Restart) == "" {
		return false
	}
	if newHash == "" || newHash == prevHash {
		return false
	}
	if opts.DryRun {
		fmt.Fprintf(
			w,
			"  Package %s: [DRY RUN] would restart service: %s\n",
			name,
			pkg.Service.Restart,
		)
		return false
	}
	fmt.Fprintf(w, "  Package %s: installed binary changed, restarting service\n", name)
	timeout := time.Duration(pkg.Timeout) * time.Second
	if timeout == 0 {
		timeout = config.DefaultExecTimeout
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(rctx, "sh", "-c", pkg.Service.Restart).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		fmt.Fprintf(os.Stderr, "  Package %s: service restart failed (continuing): %v\n", name, err)
		if msg != "" {
			fmt.Fprintf(os.Stderr, "    %s\n", msg)
		}
		return false
	}
	return true
}

func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
