package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/dotter/internal/config"
)

// BuildState tracks the completion status of builds with run = "once"
type BuildState struct {
	Builds map[string]BuildRecord `json:"builds"`
}

// BuildRecord holds information about a completed build
type BuildRecord struct {
	CompletedAt time.Time `json:"completed_at"`
	GitHash     string    `json:"git_hash,omitempty"` // Git commit hash at time of build
}

// getStateFilePath returns the path to the builds state file
func getStateFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "dotter", ".builds_state"), nil
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

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// BuildOptions holds options for running builds
type BuildOptions struct {
	DryRun        bool
	Force         bool   // Force re-run of "once" builds
	SpecificBuild string // Run only this specific build (empty = run all applicable)
}

// getGitHash returns the current git commit hash for a directory
// Returns empty string if not a git repository or git is not available
func getGitHash(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// hasGitChanges checks if the working directory has uncommitted changes
func hasGitChanges(dir string) bool {
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
func RunBuild(name string, build config.Build, opts BuildOptions) error {
	// Expand working directory early (needed for git hash check)
	workingDir := ""
	if build.WorkingDir != "" {
		var err error
		workingDir, err = config.ExpandPath(build.WorkingDir)
		if err != nil {
			return fmt.Errorf("failed to expand working directory '%s': %w", build.WorkingDir, err)
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
					currentHash := getGitHash(workingDir)
					if currentHash != "" && currentHash != record.GitHash {
						fmt.Printf("  Build '%s' has git changes (was: %s, now: %s). Re-running.\n",
							name, record.GitHash[:8], currentHash[:8])
						// Continue to run the build
					} else if hasGitChanges(workingDir) {
						fmt.Printf("  Build '%s' has uncommitted changes. Re-running.\n", name)
						// Continue to run the build
					} else {
						fmt.Printf("  Build '%s' already completed (run=once). Skipping.\n", name)
						return nil
					}
				} else {
					fmt.Printf("  Build '%s' already completed (run=once). Skipping.\n", name)
					return nil
				}
			}
		}
	case "manual":
		// Manual builds only run when explicitly requested
		if opts.SpecificBuild != name {
			fmt.Printf("  Build '%s' is manual. Skipping (use --build=%s to run).\n", name, name)
			return nil
		}
	default:
		return fmt.Errorf("unknown run mode '%s' for build '%s'", build.Run, name)
	}

	fmt.Printf("  Running build: %s\n", name)

	// Execute each command
	for i, cmdStr := range build.Commands {
		if opts.DryRun {
			if workingDir != "" {
				fmt.Printf("    [DRY RUN] Would run in '%s': %s\n", workingDir, cmdStr)
			} else {
				fmt.Printf("    [DRY RUN] Would run: %s\n", cmdStr)
			}
			continue
		}

		fmt.Printf("    [%d/%d] %s\n", i+1, len(build.Commands), cmdStr)

		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if workingDir != "" {
			cmd.Dir = workingDir
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %s: %w", cmdStr, err)
		}
	}

	// Mark build as completed for "once" mode
	if !opts.DryRun && build.Run == "once" {
		state, err := LoadBuildState()
		if err != nil {
			return fmt.Errorf("failed to load build state: %w", err)
		}
		record := BuildRecord{
			CompletedAt: time.Now(),
		}
		// Store git hash if working directory is a git repo
		if workingDir != "" {
			if hash := getGitHash(workingDir); hash != "" {
				record.GitHash = hash
			}
		}
		state.Builds[name] = record
		if err := SaveBuildState(state); err != nil {
			return fmt.Errorf("failed to save build state: %w", err)
		}
	}

	return nil
}

// RunBuilds executes all build hooks that should run
func RunBuilds(builds map[string]config.Build, opts BuildOptions) error {
	if len(builds) == 0 {
		return nil
	}

	fmt.Println("\nProcessing builds...")

	// If a specific build is requested, only run that one
	if opts.SpecificBuild != "" {
		build, exists := builds[opts.SpecificBuild]
		if !exists {
			return fmt.Errorf("build '%s' not found in configuration", opts.SpecificBuild)
		}
		if err := RunBuild(opts.SpecificBuild, build, opts); err != nil {
			return fmt.Errorf("build '%s' failed: %w", opts.SpecificBuild, err)
		}
		return nil
	}

	// Run all applicable builds
	for name, build := range builds {
		if err := RunBuild(name, build, opts); err != nil {
			return fmt.Errorf("build '%s' failed: %w", name, err)
		}
	}
	return nil
}
