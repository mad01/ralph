package hooks

import (
	"bytes"
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

// RunBuild executes a build hook
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

	// Expand working directory early (needed for git hash check)
	workingDir := ""
	if build.WorkingDir != "" {
		var err error
		workingDir, err = config.ExpandPath(build.WorkingDir)
		if err != nil {
			return fmt.Errorf("failed to expand working directory '%s': %w", build.WorkingDir, err)
		}
	}

	// Idempotent short-circuit: if the build is flagged idempotent and the
	// stored content hash matches, skip without running.
	if build.Idempotent && !opts.Force {
		hash := computeBuildHash(name, build.Commands, build.Script, workingDir)
		state, err := LoadBuildState()
		if err != nil {
			return fmt.Errorf("failed to load build state: %w", err)
		}
		if record, exists := state.Builds[name]; exists && record.ContentHash == hash {
			fmt.Fprintf(w, "  Build '%s' content unchanged. Skipping (idempotent).\n", name)
			return nil
		}
	}

	// Check run mode
	switch build.Run {
	case "always":
		// Always run
	case "once":
		if !opts.Force {
			state, err := LoadBuildState()
			if err != nil {
				return fmt.Errorf("failed to load build state: %w", err)
			}
			if record, exists := state.Builds[name]; exists {
				if workingDir != "" && record.GitHash != "" {
					// Freshness is keyed on the tree hash of working_dir's
					// subtree, not the repo-wide commit — so commits elsewhere
					// in the repo (e.g. unrelated dotfiles edits) don't force a
					// rebuild. Records from older versions hold a commit hash;
					// the first run after upgrading rebuilds once, then stores
					// the tree hash.
					currentHash := GetTreeHash(workingDir)
					if currentHash != "" && currentHash != record.GitHash {
						fmt.Fprintf(
							w,
							"  Build '%s' has source changes (was: %s, now: %s). Re-running.\n",
							name,
							shortHash(record.GitHash),
							shortHash(currentHash),
						)
					} else if HasGitChangesInPath(workingDir) {
						fmt.Fprintf(w, "  Build '%s' has uncommitted changes. Re-running.\n", name)
					} else {
						fmt.Fprintf(w, "  Build '%s' already completed (run=once). Skipping.\n", name)
						return nil
					}
				} else {
					fmt.Fprintf(w, "  Build '%s' already completed (run=once). Skipping.\n", name)
					return nil
				}
			}
		}
	case "manual":
		if opts.SpecificBuild != name {
			fmt.Fprintf(
				w,
				"  Build '%s' is manual. Skipping (use --build=%s to run).\n",
				name,
				name,
			)
			return nil
		}
	default:
		return fmt.Errorf("unknown run mode '%s' for build '%s'", build.Run, name)
	}

	if build.Script != "" && len(build.Commands) > 0 {
		return fmt.Errorf("build '%s': script and commands are mutually exclusive", name)
	}

	fmt.Fprintf(w, "  Running build: %s\n", name)

	timeout := time.Duration(build.Timeout) * time.Second
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

	if build.Script != "" {
		scriptPath := build.Script
		if !filepath.IsAbs(scriptPath) {
			base := workingDir
			if base == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("build '%s': could not get working directory: %w", name, err)
				}
			}
			scriptPath = filepath.Join(base, scriptPath)
		}

		if opts.DryRun {
			if workingDir != "" {
				fmt.Fprintf(
					w,
					"    [DRY RUN] Would run script in '%s': %s\n",
					workingDir,
					scriptPath,
				)
			} else {
				fmt.Fprintf(w, "    [DRY RUN] Would run script: %s\n", scriptPath)
			}
		} else {
			fmt.Fprintf(w, "    Running script: %s\n", scriptPath)
			cmd := execx.Command(ctx, "sh", scriptPath)
			cmd.Stdout = w
			cmd.Stderr = stderrW
			if workingDir != "" {
				cmd.Dir = workingDir
			}
			if err := cmd.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Errorf("build '%s' timed out after %s: script %s", name, timeout, scriptPath)
				}
				if !opts.Verbose && stderrBuf.Len() > 0 {
					_, _ = os.Stderr.Write(stderrBuf.Bytes())
				}
				return fmt.Errorf("script failed: %s: %w", scriptPath, err)
			}
		}
	} else {
		for i, cmdStr := range build.Commands {
			if opts.DryRun {
				if workingDir != "" {
					fmt.Fprintf(w, "    [DRY RUN] Would run in '%s': %s\n", workingDir, cmdStr)
				} else {
					fmt.Fprintf(w, "    [DRY RUN] Would run: %s\n", cmdStr)
				}
				continue
			}

			fmt.Fprintf(w, "    [%d/%d] %s\n", i+1, len(build.Commands), cmdStr)

			cmd := execx.Command(ctx, "sh", "-c", cmdStr)
			cmd.Stdout = w
			cmd.Stderr = stderrW
			if workingDir != "" {
				cmd.Dir = workingDir
			}

			if err := cmd.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Errorf("build '%s' timed out after %s: %s", name, timeout, cmdStr)
				}
				if !opts.Verbose && stderrBuf.Len() > 0 {
					_, _ = os.Stderr.Write(stderrBuf.Bytes())
				}
				return fmt.Errorf("command failed: %s: %w", cmdStr, err)
			}
		}
	}

	if !opts.DryRun && (build.Run == "once" || build.Idempotent) {
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
