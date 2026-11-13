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

	"github.com/mad01/ralph/internal/buildstate"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/execx"
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

// SyncAction is what SyncPackages did with one package.
type SyncAction string

const (
	SyncCloned  SyncAction = "cloned"
	SyncPulled  SyncAction = "pulled"
	SyncSkipped SyncAction = "skipped"
	SyncError   SyncAction = "error"
)

// SyncResult holds the result of syncing a single package.
type SyncResult struct {
	Name    string
	Action  SyncAction
	Message string
	Err     error
}

// BuildOptions holds options for building packages.
type BuildOptions struct {
	DryRun  bool
	Force   bool
	Verbose bool
}

// BuildAction is what BuildPackage did with one package.
type BuildAction string

const (
	ActionBuilt    BuildAction = "built"
	ActionUpToDate BuildAction = "up-to-date"
	ActionSkipped  BuildAction = "skipped"
	ActionError    BuildAction = "error"
)

// BuildResult holds the result of building a single package.
type BuildResult struct {
	Name             string
	Action           BuildAction
	Message          string
	Err              error
	ServiceRestarted bool // true when a package [service] restart command ran after the binary changed
}

// ResolvePackagePaths resolves default target and working_dir for a package.
func ResolvePackagePaths(name string, pkg config.Package, packagesDir string) config.Package {
	if packagesDir == "" {
		if dir, err := config.DefaultPackagesDir(); err == nil {
			packagesDir = dir
		}
	}

	resolved := pkg

	// go-install packages don't need target or working_dir resolution
	if pkg.Source == config.SourceGoInstall {
		return resolved
	}

	if pkg.Source.Clones() {
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
	machineProfiles []string,
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

		source := pkg.Source.OrLocal()

		reason := config.SkipReason(
			pkg.Enable,
			pkg.Hosts,
			pkg.Profiles,
			currentHost,
			machineProfiles,
		)
		if reason != "" {
			fmt.Fprintf(w, "  Skipping package: %s [%s] (%s)\n", name, source, reason)
			results = append(
				results,
				SyncResult{
					Name:    name,
					Action:  SyncSkipped,
					Message: fmt.Sprintf("%s [%s]", reason, source),
				},
			)
			continue
		}

		if source == config.SourceGoInstall {
			fmt.Fprintf(w, "  Skipping package: %s [go-install] (nothing to sync)\n", name)
			results = append(
				results,
				SyncResult{
					Name:    name,
					Action:  SyncSkipped,
					Message: "go-install package (nothing to sync)",
				},
			)
			continue
		}

		if source == config.SourceLocal {
			fmt.Fprintf(w, "  Skipping package: %s [local] (nothing to sync)\n", name)
			results = append(
				results,
				SyncResult{Name: name, Action: SyncSkipped, Message: "local package"},
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
			return SyncResult{Name: name, Action: SyncError, Message: "clone failed", Err: err}
		}
		if opts.DryRun {
			return SyncResult{Name: name, Action: SyncCloned, Message: "[DRY RUN] would clone"}
		}
		return SyncResult{Name: name, Action: SyncCloned, Message: "cloned"}
	}

	fmt.Fprintf(w, "  Package %s [remote]: pulling latest...\n", name)
	if err := GitPull(ctx, w, target, opts.DryRun, opts.Verbose); err != nil {
		return SyncResult{Name: name, Action: SyncError, Message: "pull failed", Err: err}
	}

	if opts.DryRun {
		return SyncResult{Name: name, Action: SyncPulled, Message: "[DRY RUN] would pull"}
	}
	return SyncResult{Name: name, Action: SyncPulled, Message: "pulled"}
}

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

// BuildPackage builds and installs a single package when DecidePackage says
// it needs it. The package paths should already be resolved via
// ResolvePackagePaths before calling.
func BuildPackage(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts BuildOptions,
) BuildResult {
	if pkg.Source == config.SourceGoInstall {
		return buildGoInstallPackage(ctx, w, name, pkg, opts)
	}
	source := pkg.Source.OrLocal()

	in := FreshnessInput{Package: pkg, Force: opts.Force}
	statErr := statWorkDir(pkg.WorkingDir)
	in.WorkDirExists = statErr == nil
	if in.WorkDirExists && !opts.DryRun {
		in.TreeHash, in.Dirty = sourceState(pkg.WorkingDir)
		if !opts.Force {
			in.Record = loadRecord(stateKey(name))
			in.Installed = installedCheck(pkg, in.Record)
		}
	}
	d := DecidePackage(in)
	if res, unbuildable := unbuildableResult(name, pkg.WorkingDir, d, statErr); unbuildable {
		return res
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
			Action:  ActionBuilt,
			Message: "[DRY RUN] would check and rebuild if changed",
		}
	}

	reportPackageDecision(w, name, source, d)
	if !d.NeedsBuild {
		return BuildResult{Name: name, Action: ActionUpToDate, Message: "no changes detected"}
	}
	return buildAndInstall(ctx, w, name, pkg, opts)
}

// stateKey is a package's key in the build state.
func stateKey(name string) string {
	return "pkg:" + name
}

// loadRecord returns a package's build record, nil when there is none or the
// build state is unreadable (which loadBuildState reports once).
func loadRecord(key string) *buildstate.BuildRecord {
	state, err := loadBuildState()
	if err != nil {
		return nil
	}
	if rec, ok := state.Builds[key]; ok {
		return &rec
	}
	return nil
}

// unbuildableResult returns the result for a package whose working dir is
// missing: a remote one is skipped until sync clones it, a local one fails.
func unbuildableResult(name, workDir string, d Freshness, statErr error) (BuildResult, bool) {
	switch d.Cause {
	case CauseNotCloned:
		return BuildResult{
			Name:    name,
			Action:  ActionSkipped,
			Message: "not cloned (run 'ralph sync' first)",
		}, true
	case CauseWorkDirMissing:
		return failed(name, fmt.Sprintf("working_dir '%s' does not exist", workDir), statErr), true
	}
	return BuildResult{}, false
}

// reportPackageDecision prints the line for a decision that has one. A
// package without a record, or with an unreadable build state, builds
// without a line.
func reportPackageDecision(w io.Writer, name string, source config.PackageSource, d Freshness) {
	prefix := fmt.Sprintf("  Package %s [%s]: ", name, source)
	switch d.Cause {
	case CauseNoSourceHash:
		fmt.Fprintln(w, prefix+"no recorded source hash, rebuilding to repair state")
	case CauseSourceChanged:
		fmt.Fprintf(w, "%sgit changes detected (%s → %s)\n", prefix, short(d.Was), short(d.Now))
	case CauseUncommitted:
		fmt.Fprintln(w, prefix+"uncommitted changes detected")
	case CauseInstalled:
		fmt.Fprintln(w, prefix+d.Reason+", rebuilding")
	case CauseUpToDate:
		fmt.Fprintln(w, prefix+"up to date")
	case CauseForced:
		fmt.Fprintln(w, prefix+"force rebuild")
	}
}

// failed is an error BuildResult.
func failed(name, message string, err error) BuildResult {
	return BuildResult{Name: name, Action: ActionError, Message: message, Err: err}
}

// packageCommands returns a package's build and install commands, with the
// make defaults for a source = "make" package that leaves them empty.
func packageCommands(pkg config.Package) (build, install []string) {
	build, install = pkg.Build, pkg.Install
	if pkg.Source == config.SourceMake {
		if len(build) == 0 {
			build = []string{"make build"}
		}
		if len(install) == 0 {
			install = []string{"make install"}
		}
	}
	return build, install
}

// buildAndInstall runs a package's build and install commands, validates the
// installed artifacts, records the build and restarts its service when the
// installed content changed.
func buildAndInstall(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts BuildOptions,
) BuildResult {
	key := stateKey(name)
	build, install := packageCommands(pkg)
	prevInstallHash := loadInstallHash(key)

	if err := runCommands(ctx, w, "build", build, pkg, opts); err != nil {
		return failed(name, "build failed", err)
	}
	if err := runCommands(ctx, w, "install", install, pkg, opts); err != nil {
		return failed(name, "install failed", err)
	}
	if err := validateInstalledVersion(pkg, pkg.WorkingDir); err != nil {
		return failed(name, "installed version validation failed", err)
	}
	newInstallHash, err := validatedInstallHash(pkg)
	if err != nil {
		return failed(name, "installed artifact validation failed", err)
	}
	savePackageState(w, key, pkg.WorkingDir, newInstallHash)
	result := BuildResult{Name: name, Action: ActionBuilt, Message: "rebuilt"}
	if maybeRestartService(ctx, w, name, pkg, prevInstallHash, newInstallHash, opts) {
		result.ServiceRestarted = true
		result.Message = "rebuilt; service restarted"
	}
	return result
}

// buildGoInstallPackage installs a go-install package when DecidePackage
// says its version or installed artifacts changed.
func buildGoInstallPackage(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts BuildOptions,
) BuildResult {
	source := config.SourceGoInstall
	if opts.DryRun {
		cmdStr := goInstallCommand(pkg, dryRunGOBIN(pkg))
		fmt.Fprintf(w, "  Package %s [%s]: [DRY RUN] would run: %s\n", name, source, cmdStr)
		return BuildResult{Name: name, Action: ActionBuilt, Message: "[DRY RUN] would go install"}
	}

	in := FreshnessInput{Package: pkg, Force: opts.Force}
	if !opts.Force {
		in.Record = loadRecord(stateKey(name))
		in.Installed = installedCheck(pkg, in.Record)
	}
	switch d := DecidePackage(in); d.Cause {
	case CauseInstalled:
		fmt.Fprintf(w, "  Package %s [%s]: %s, reinstalling\n", name, source, d.Reason)
	case CauseUpToDate:
		fmt.Fprintf(w, "  Package %s [%s]: up to date (%s)\n", name, source, pkg.Version)
		return BuildResult{
			Name:    name,
			Action:  ActionUpToDate,
			Message: fmt.Sprintf("version %s already installed", pkg.Version),
		}
	}
	return goInstall(ctx, w, name, pkg, opts)
}

// goInstallCommand is the shell command that installs a go-install package.
func goInstallCommand(pkg config.Package, gobin string) string {
	return fmt.Sprintf("GOBIN=%s go install %s@%s", gobin, pkg.Module, pkg.Version)
}

// dryRunGOBIN is the GOBIN a dry run prints: the first install_path's
// directory, "" when it has none or it does not expand.
func dryRunGOBIN(pkg config.Package) string {
	if len(pkg.InstallPaths) == 0 {
		return ""
	}
	expanded, err := config.ExpandPath(pkg.InstallPaths[0])
	if err != nil {
		return ""
	}
	return filepath.Dir(expanded)
}

// goInstall runs go install into the first install_path's directory,
// records the version and the installed content, and restarts the service
// when that content changed. The restart shares the install's timeout.
func goInstall(
	ctx context.Context,
	w io.Writer,
	name string,
	pkg config.Package,
	opts BuildOptions,
) BuildResult {
	if _, err := exec.LookPath("go"); err != nil {
		return failed(name, "go not found in PATH", fmt.Errorf("go command not available: %w", err))
	}
	gobin, msg, err := prepareGOBIN(pkg)
	if err != nil {
		return failed(name, msg, err)
	}

	cmdStr := goInstallCommand(pkg, gobin)
	fmt.Fprintf(w, "  Package %s [%s]: %s\n", name, config.SourceGoInstall, cmdStr)

	timeout := config.ExecTimeout(pkg.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sh := &execx.Shell{Out: w, Verbose: opts.Verbose}
	if err := sh.Run(ctx, "-c", cmdStr); err != nil {
		if execx.TimedOut(ctx) {
			return failed(name, fmt.Sprintf("go install timed out after %s", timeout), err)
		}
		return failed(name, "go install failed", err)
	}

	key := stateKey(name)
	prevInstallHash := loadInstallHash(key)
	newInstallHash, msg, err := recordGoInstall(key, pkg)
	if err != nil {
		return failed(name, msg, err)
	}
	result := BuildResult{
		Name:    name,
		Action:  ActionBuilt,
		Message: fmt.Sprintf("installed %s@%s", pkg.Module, pkg.Version),
	}
	if maybeRestartService(ctx, w, name, pkg, prevInstallHash, newInstallHash, opts) {
		result.ServiceRestarted = true
		result.Message += "; service restarted"
	}
	return result
}

// prepareGOBIN returns the GOBIN for a go-install package, the first
// install_path's directory, and creates it. "" leaves GOBIN to go. On error
// it also returns the result message.
func prepareGOBIN(pkg config.Package) (gobin, msg string, err error) {
	if len(pkg.InstallPaths) == 0 {
		return "", "", nil
	}
	expanded, err := config.ExpandPath(pkg.InstallPaths[0])
	if err != nil {
		return "", "failed to expand install_path", err
	}
	gobin = filepath.Dir(expanded)
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		return "", "failed to create GOBIN directory", err
	}
	return gobin, "", nil
}

// recordGoInstall records a go-install package's version and installed
// content hash, and returns that hash. On error it also returns the result
// message.
func recordGoInstall(key string, pkg config.Package) (installHash, msg string, err error) {
	installHash, err = validatedInstallHash(pkg)
	if err != nil {
		return "", "installed artifact validation failed", err
	}
	state, err := buildstate.LoadBuildState()
	if err != nil {
		return "", "failed to load build state", err
	}
	if state.Builds == nil {
		state.Builds = make(map[string]buildstate.BuildRecord)
	}
	state.Builds[key] = buildstate.BuildRecord{
		CompletedAt: time.Now(),
		Version:     pkg.Version,
		InstallHash: installHash,
	}
	if err := buildstate.SaveBuildState(state); err != nil {
		return "", "failed to save build state", err
	}
	return installHash, "", nil
}

// runCommands runs one phase (label "build" or "install") of a package's
// commands in its working dir under the package timeout, stopping at the
// first failure.
func runCommands(
	ctx context.Context,
	w io.Writer,
	label string,
	commands []string,
	pkg config.Package,
	opts BuildOptions,
) error {
	if len(commands) == 0 {
		return nil
	}
	timeout := config.ExecTimeout(pkg.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sh := &execx.Shell{Out: w, Dir: pkg.WorkingDir, Verbose: opts.Verbose}
	for i, cmdStr := range commands {
		if opts.DryRun {
			fmt.Fprintf(w, "    [DRY RUN] Would %s in '%s': %s\n", label, pkg.WorkingDir, cmdStr)
			continue
		}

		fmt.Fprintf(w, "    [%s %d/%d] %s\n", label, i+1, len(commands), cmdStr)
		if err := sh.Run(ctx, "-c", cmdStr); err != nil {
			if execx.TimedOut(ctx) {
				return fmt.Errorf("%s timed out after %s: %s", label, timeout, cmdStr)
			}
			return fmt.Errorf("%s command failed: %s: %w", label, cmdStr, err)
		}
	}
	return nil
}

// gitTimeout bounds a git clone or pull of a package or the dotfiles repo. A
// var so tests can shorten it.
var gitTimeout = config.DefaultExecTimeout

// GitPull runs git pull in the given directory.
// If the current branch has no upstream tracking, it falls back to
// pulling from origin with the current branch name.
func GitPull(ctx context.Context, w io.Writer, dir string, dryRun, verbose bool) error {
	if dryRun {
		fmt.Fprintf(w, "    [DRY RUN] Would run: git pull in %s\n", dir)
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	// Always capture stderr to a buffer so the no-tracking fallback below can
	// inspect it; in verbose mode also stream it to os.Stderr live.
	var stderrBuf bytes.Buffer
	stderrW := io.Writer(&stderrBuf)
	if verbose {
		stderrW = io.MultiWriter(os.Stderr, &stderrBuf)
	}

	cmd := execx.TerminalCommand(ctx, "git", "pull")
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull timed out after %s in %s", gitTimeout, dir)
		}
		// If pull failed due to no tracking info, try pulling current branch from origin
		if stderrBuf.Len() > 0 && strings.Contains(stderrBuf.String(), "no tracking information") {
			branch := getCurrentBranch(ctx, dir)
			if branch != "" {
				stderrBuf.Reset()
				fmt.Fprintf(w, "    No tracking info, pulling origin/%s...\n", branch)
				retryCmd := execx.TerminalCommand(ctx, "git", "pull", "origin", branch)
				retryCmd.Dir = dir
				retryCmd.Stdout = w
				retryCmd.Stderr = stderrW
				retryErr := retryCmd.Run()
				if retryErr == nil {
					return nil
				}
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Errorf("git pull timed out after %s in %s", gitTimeout, dir)
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
func getCurrentBranch(ctx context.Context, dir string) string {
	cmd := execx.TerminalCommand(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
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

	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
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

	cmd := execx.TerminalCommand(ctx, "git", args...)
	cmd.Stdout = w
	cmd.Stderr = stderrW
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone timed out after %s: %s", gitTimeout, url)
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
	state, err := loadBuildState()
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
	timeout := config.ExecTimeout(pkg.Timeout)
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := execx.Command(rctx, "sh", "-c", pkg.Service.Restart).CombinedOutput()
	if err != nil {
		if rctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("timed out after %s", timeout)
		}
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
