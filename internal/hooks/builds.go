package hooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/progress"
)

// BuildState tracks the completion status of builds with run = "once"
type BuildState struct {
	Builds map[string]BuildRecord `json:"builds"`
}

// BuildRecord holds information about a completed build
type BuildRecord struct {
	CompletedAt time.Time `json:"completed_at"`
	GitHash     string    `json:"git_hash,omitempty"`     // Git commit hash at time of build
	ContentHash string    `json:"content_hash,omitempty"` // Hash of (name, commands, working_dir) for idempotent skip
}

// computeBuildHash returns a stable hex-encoded sha256 over the build's
// identity (name + commands + script + working_dir). Used to short-circuit
// idempotent builds when their content hasn't changed since the last successful run.
func computeBuildHash(name string, commands []string, script string, workingDir string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	for _, c := range commands {
		h.Write([]byte(c))
		h.Write([]byte{0})
	}
	h.Write([]byte(script))
	h.Write([]byte{0})
	h.Write([]byte(workingDir))
	return hex.EncodeToString(h.Sum(nil))
}

// getStateFilePath returns the path to the builds state file
func getStateFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "ralph", ".builds_state"), nil
}

// LoadBuildState loads the build state from the state file
func LoadBuildState() (*BuildState, error) {
	statePath, err := getStateFilePath()
	if err != nil {
		return nil, err
	}

	state := &BuildState{
		Builds: make(map[string]BuildRecord),
	}

	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return state, nil
}

// SaveBuildState saves the build state to the state file
func SaveBuildState(state *BuildState) error {
	statePath, err := getStateFilePath()
	if err != nil {
		return err
	}

	// Ensure the directory exists
	stateDir := filepath.Dir(statePath)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp build state file: %w", err)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming build state file: %w", err)
	}

	return nil
}

// BuildOptions holds options for running builds
type BuildOptions struct {
	DryRun        bool
	Force         bool   // Force re-run of "once" builds
	SpecificBuild string // Run only this specific build (empty = run all applicable)
	Verbose       bool
}

// GetGitHash returns the current git commit hash for a directory.
// Returns empty string if not a git repository or git is not available.
func GetGitHash(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// HasGitChanges checks if the working directory has uncommitted changes.
func HasGitChanges(dir string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// ResetBuildState clears all build state
func ResetBuildState() error {
	statePath, err := getStateFilePath()
	if err != nil {
		return err
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}

	fmt.Println("Build state has been reset.")
	return nil
}

// ResetBuildStateForName clears the build state for a specific build
func ResetBuildStateForName(name string) error {
	state, err := LoadBuildState()
	if err != nil {
		return err
	}

	if _, exists := state.Builds[name]; exists {
		delete(state.Builds, name)
		if err := SaveBuildState(state); err != nil {
			return err
		}
		fmt.Printf("Build state for '%s' has been reset.\n", name)
	}
	return nil
}

// RunBuild executes a build hook
func RunBuild(w io.Writer, name string, build config.Build, currentHost string, opts BuildOptions) error {
	// Check enable first
	if !config.IsEnabled(build.Enable) {
		fmt.Fprintf(w, "  Skipping build: %s (disabled)\n", name)
		return nil
	}

	// Check host filter
	if !config.ShouldApplyForHost(build.Hosts, currentHost) {
		fmt.Fprintf(w, "  Skipping build: %s (host filter)\n", name)
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
	// stored content hash matches, skip without running. Applies to all run
	// modes — "always" + idempotent means "run only when content changes".
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
				// Check if git hash has changed (if we have a working dir and recorded hash)
				if workingDir != "" && record.GitHash != "" {
					currentHash := GetGitHash(workingDir)
					if currentHash != "" && currentHash != record.GitHash {
						fmt.Fprintf(w, "  Build '%s' has git changes (was: %s, now: %s). Re-running.\n",
							name, record.GitHash[:8], currentHash[:8])
						// Continue to run the build
					} else if HasGitChanges(workingDir) {
						fmt.Fprintf(w, "  Build '%s' has uncommitted changes. Re-running.\n", name)
						// Continue to run the build
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
		// Manual builds only run when explicitly requested
		if opts.SpecificBuild != name {
			fmt.Fprintf(w, "  Build '%s' is manual. Skipping (use --build=%s to run).\n", name, name)
			return nil
		}
	default:
		return fmt.Errorf("unknown run mode '%s' for build '%s'", build.Run, name)
	}

	// Validate mutual exclusivity at runtime (belt-and-suspenders; validation should catch this first)
	if build.Script != "" && len(build.Commands) > 0 {
		return fmt.Errorf("build '%s': script and commands are mutually exclusive", name)
	}

	fmt.Fprintf(w, "  Running build: %s\n", name)

	// Set up timeout context
	timeout := time.Duration(build.Timeout) * time.Second
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

	if build.Script != "" {
		// Resolve script path: relative to working_dir if set, otherwise current directory
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
				fmt.Fprintf(w, "    [DRY RUN] Would run script in '%s': %s\n", workingDir, scriptPath)
			} else {
				fmt.Fprintf(w, "    [DRY RUN] Would run script: %s\n", scriptPath)
			}
		} else {
			fmt.Fprintf(w, "    Running script: %s\n", scriptPath)
			cmd := exec.CommandContext(ctx, "sh", scriptPath)
			cmd.Stdout = w
			cmd.Stderr = stderrW
			if workingDir != "" {
				cmd.Dir = workingDir
			}
			if err := cmd.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Errorf("build '%s' timed out after %ds: script %s", name, build.Timeout, scriptPath)
				}
				if !opts.Verbose && stderrBuf.Len() > 0 {
					os.Stderr.Write(stderrBuf.Bytes())
				}
				return fmt.Errorf("script failed: %s: %w", scriptPath, err)
			}
		}
	} else {
		// Execute each command
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

			cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
			cmd.Stdout = w
			cmd.Stderr = stderrW
			if workingDir != "" {
				cmd.Dir = workingDir
			}

			if err := cmd.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return fmt.Errorf("build '%s' timed out after %ds: %s", name, build.Timeout, cmdStr)
				}
				if !opts.Verbose && stderrBuf.Len() > 0 {
					os.Stderr.Write(stderrBuf.Bytes())
				}
				return fmt.Errorf("command failed: %s: %w", cmdStr, err)
			}
		}
	}

	// Persist state for "once" runs and for any idempotent build, so the
	// content-hash short-circuit works on the next apply.
	if !opts.DryRun && (build.Run == "once" || build.Idempotent) {
		state, err := LoadBuildState()
		if err != nil {
			return fmt.Errorf("failed to load build state: %w", err)
		}
		record := state.Builds[name]
		record.CompletedAt = time.Now()
		if workingDir != "" {
			if hash := GetGitHash(workingDir); hash != "" {
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

// BuildResult captures the outcome of a single build hook.
type BuildResult struct {
	Name string
	Err  error
}

// RunBuilds executes all build hooks that should run.
// Failures are collected and reported — a single failing build does not
// prevent remaining builds from executing.
func RunBuilds(w io.Writer, builds map[string]config.Build, currentHost string, opts BuildOptions) error {
	if len(builds) == 0 {
		return nil
	}

	fmt.Fprintln(w, "\nProcessing builds...")

	// If a specific build is requested, only run that one
	if opts.SpecificBuild != "" {
		build, exists := builds[opts.SpecificBuild]
		if !exists {
			return fmt.Errorf("build '%s' not found in configuration", opts.SpecificBuild)
		}
		if err := RunBuild(w, opts.SpecificBuild, build, currentHost, opts); err != nil {
			return fmt.Errorf("build '%s' failed: %w", opts.SpecificBuild, err)
		}
		return nil
	}

	// Run all applicable builds in sorted order for deterministic execution
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
		if err := RunBuild(w, name, builds[name], currentHost, opts); err != nil {
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
