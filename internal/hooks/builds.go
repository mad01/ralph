package hooks

import (
	"context"
	"fmt"
	"io"
	"os"
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

// Type aliases so existing callers (commands, tests) keep compiling.
type (
	BuildState  = buildstate.BuildState
	BuildRecord = buildstate.BuildRecord
)

// Delegate state operations to buildstate package.
var (
	LoadBuildState         = buildstate.LoadBuildState
	SaveBuildState         = buildstate.SaveBuildState
	ResetBuildState        = buildstate.ResetBuildState
	ResetBuildStateForName = buildstate.ResetBuildStateForName
)

// Delegate git operations to gitutil package.
var (
	GetTreeHash         = gitutil.GetTreeHash
	HasGitChangesInPath = gitutil.HasGitChangesInPath
)

var computeBuildHash = buildstate.ComputeBuildHash

// BuildOptions holds options for running builds
type BuildOptions struct {
	DryRun        bool
	Force         bool   // Force re-run of "once" builds
	SpecificBuild string // Run only this specific build (empty = run all applicable)
	Verbose       bool
}

// RunBuild executes a build hook: it applies the enable, host and profile
// filters, asks DecideBuild whether the build runs, runs its script or
// commands, and records a run = "once" or idempotent build's success.
func RunBuild(
	ctx context.Context,
	w io.Writer,
	name string,
	build config.Build,
	currentHost string,
	machineProfiles []string,
	opts BuildOptions,
) error {
	reason := config.SkipReason(
		build.Enable,
		build.Hosts,
		build.Profiles,
		currentHost,
		machineProfiles,
	)
	if reason != "" {
		fmt.Fprintf(w, "  Skipping build: %s (%s)\n", name, reason)
		return nil
	}
	if build.Script != "" && len(build.Commands) > 0 {
		return fmt.Errorf("build '%s': script and commands are mutually exclusive", name)
	}
	workingDir, err := expandWorkingDir(build.WorkingDir)
	if err != nil {
		return err
	}

	decision, err := decideFromState(name, build, workingDir, opts)
	if err != nil {
		return err
	}
	reportBuildDecision(w, name, decision)
	if !decision.Run {
		return nil
	}

	fmt.Fprintf(w, "  Running build: %s\n", name)
	if err := runBuildSteps(ctx, w, name, build, workingDir, opts); err != nil {
		return err
	}
	if opts.DryRun || (build.Run != config.RunOnce && !build.Idempotent) {
		return nil
	}
	return recordBuild(name, build, workingDir)
}

// expandWorkingDir expands a build's working_dir; "" stays "".
func expandWorkingDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	expanded, err := config.ExpandPath(dir)
	if err != nil {
		return "", fmt.Errorf("failed to expand working directory '%s': %w", dir, err)
	}
	return expanded, nil
}

// decideFromState asks DecideBuild with facts read from the build-state file
// and the working dir.
func decideFromState(
	name string,
	build config.Build,
	workingDir string,
	opts BuildOptions,
) (BuildDecision, error) {
	return DecideBuild(BuildInput{
		Name:          name,
		Build:         build,
		WorkingDir:    workingDir,
		Force:         opts.Force,
		SpecificBuild: opts.SpecificBuild,
		Facts:         StateFacts{Name: name, WorkingDir: workingDir},
	})
}

// reportBuildDecision prints the line for a decision that has one. A build
// that runs because of its run mode alone prints nothing here.
func reportBuildDecision(w io.Writer, name string, d BuildDecision) {
	switch d.Reason {
	case ReasonContentUnchanged:
		fmt.Fprintf(w, "  Build '%s' content unchanged. Skipping (idempotent).\n", name)
	case ReasonSourceChanged:
		fmt.Fprintf(
			w,
			"  Build '%s' has source changes (was: %s, now: %s). Re-running.\n",
			name,
			shortHash(d.WasHash),
			shortHash(d.NowHash),
		)
	case ReasonUncommitted:
		fmt.Fprintf(w, "  Build '%s' has uncommitted changes. Re-running.\n", name)
	case ReasonCompleted:
		fmt.Fprintf(w, "  Build '%s' already completed (run=once). Skipping.\n", name)
	case ReasonManual:
		fmt.Fprintf(w, "  Build '%s' is manual. Skipping (use --build=%s to run).\n", name, name)
	}
}

// buildRun runs one build's steps under a single timeout and one Shell, so
// stderr kept from an earlier step is replayed with a later failure.
type buildRun struct {
	name    string
	w       io.Writer
	sh      *execx.Shell
	timeout time.Duration
	dryRun  bool
}

// runBuildSteps runs a build's script, or else its commands.
func runBuildSteps(
	ctx context.Context,
	w io.Writer,
	name string,
	build config.Build,
	workingDir string,
	opts BuildOptions,
) error {
	timeout := config.ExecTimeout(build.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := buildRun{
		name:    name,
		w:       w,
		sh:      &execx.Shell{Out: w, Dir: workingDir, Verbose: opts.Verbose},
		timeout: timeout,
		dryRun:  opts.DryRun,
	}
	if build.Script != "" {
		return run.script(ctx, build.Script)
	}
	return run.commands(ctx, build.Commands)
}

// script runs a build script with sh. A relative path resolves against the
// working dir, or ralph's own when the build has none.
func (r buildRun) script(ctx context.Context, script string) error {
	scriptPath := script
	if !filepath.IsAbs(scriptPath) {
		base := r.sh.Dir
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("build '%s': could not get working directory: %w", r.name, err)
			}
		}
		scriptPath = filepath.Join(base, scriptPath)
	}

	if r.dryRun {
		if r.sh.Dir != "" {
			fmt.Fprintf(r.w, "    [DRY RUN] Would run script in '%s': %s\n", r.sh.Dir, scriptPath)
		} else {
			fmt.Fprintf(r.w, "    [DRY RUN] Would run script: %s\n", scriptPath)
		}
		return nil
	}
	fmt.Fprintf(r.w, "    Running script: %s\n", scriptPath)
	if err := r.sh.Run(ctx, scriptPath); err != nil {
		if execx.TimedOut(ctx) {
			return fmt.Errorf(
				"build '%s' timed out after %s: script %s",
				r.name,
				r.timeout,
				scriptPath,
			)
		}
		return fmt.Errorf("script failed: %s: %w", scriptPath, err)
	}
	return nil
}

// commands runs a build's commands with sh -c, stopping at the first failure.
func (r buildRun) commands(ctx context.Context, commands []string) error {
	for i, cmdStr := range commands {
		if r.dryRun {
			if r.sh.Dir != "" {
				fmt.Fprintf(r.w, "    [DRY RUN] Would run in '%s': %s\n", r.sh.Dir, cmdStr)
			} else {
				fmt.Fprintf(r.w, "    [DRY RUN] Would run: %s\n", cmdStr)
			}
			continue
		}

		fmt.Fprintf(r.w, "    [%d/%d] %s\n", i+1, len(commands), cmdStr)
		if err := r.sh.Run(ctx, "-c", cmdStr); err != nil {
			if execx.TimedOut(ctx) {
				return fmt.Errorf("build '%s' timed out after %s: %s", r.name, r.timeout, cmdStr)
			}
			return fmt.Errorf("command failed: %s: %w", cmdStr, err)
		}
	}
	return nil
}

// recordBuild records a build's success: the time, the working_dir subtree's
// tree hash, and for an idempotent build its content hash.
func recordBuild(name string, build config.Build, workingDir string) error {
	state, err := LoadBuildState()
	if err != nil {
		return fmt.Errorf("failed to load build state: %w", err)
	}
	record := state.Builds[name]
	record.CompletedAt = time.Now()
	if workingDir != "" {
		if hash := GetTreeHash(workingDir); hash != "" {
			record.GitHash = hash
		}
	}
	if build.Idempotent {
		record.ContentHash = computeBuildHash(name, build.Commands, build.Script, workingDir)
	}
	state.Builds[name] = record
	if err := SaveBuildState(state); err != nil {
		return fmt.Errorf("failed to save build state: %w", err)
	}
	return nil
}

// shortHash truncates a hash to 8 characters for one-line messages. A stored
// hash can be shorter (a hand-edited state file), so it is never sliced blind.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// BuildResult captures the outcome of a single build hook.
type BuildResult struct {
	Name string
	Err  error
}

// RunBuilds executes all build hooks that should run.
func RunBuilds(
	ctx context.Context,
	w io.Writer,
	builds map[string]config.Build,
	currentHost string,
	machineProfiles []string,
	opts BuildOptions,
) error {
	if len(builds) == 0 {
		return nil
	}

	fmt.Fprintln(w, "\nProcessing builds...")

	if opts.SpecificBuild != "" {
		build, exists := builds[opts.SpecificBuild]
		if !exists {
			return fmt.Errorf("build '%s' not found in configuration", opts.SpecificBuild)
		}
		if err := RunBuild(ctx, w, opts.SpecificBuild, build, currentHost, machineProfiles, opts); err != nil {
			return fmt.Errorf("build '%s' failed: %w", opts.SpecificBuild, err)
		}
		return nil
	}

	keys := make([]string, 0, len(builds))
	for name := range builds {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	var failures []BuildResult
	prog := progress.New("Builds", len(keys))
	if opts.Verbose || opts.DryRun {
		prog = progress.NewQuiet()
	}
	for _, name := range keys {
		prog.TickWith(name)
		if err := RunBuild(ctx, w, name, builds[name], currentHost, machineProfiles, opts); err != nil {
			failures = append(failures, BuildResult{Name: name, Err: err})
		}
	}
	prog.Done()
	for _, f := range failures {
		fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", f.Name, f.Err)
	}

	if len(failures) > 0 {
		names := make([]string, len(failures))
		for i, f := range failures {
			names[i] = f.Name
		}
		return fmt.Errorf("%d build(s) failed: %s", len(failures), strings.Join(names, ", "))
	}
	return nil
}
