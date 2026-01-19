package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// RunBuild executes a build hook
func RunBuild(name string, build config.Build, dryRun bool) error {
	// Check run mode
	switch build.Run {
	case "always":
		// Always run
	case "once":
		state, err := LoadBuildState()
		if err != nil {
			return fmt.Errorf("failed to load build state: %w", err)
		}
		if _, exists := state.Builds[name]; exists {
			fmt.Printf("  Build '%s' already completed (run=once). Skipping.\n", name)
			return nil
		}
	case "manual":
		fmt.Printf("  Build '%s' is manual. Skipping (use --build=%s to run).\n", name, name)
		return nil
	default:
		return fmt.Errorf("unknown run mode '%s' for build '%s'", build.Run, name)
	}

	fmt.Printf("  Running build: %s\n", name)

	// Expand working directory
	workingDir := ""
	if build.WorkingDir != "" {
		var err error
		workingDir, err = config.ExpandPath(build.WorkingDir)
		if err != nil {
			return fmt.Errorf("failed to expand working directory '%s': %w", build.WorkingDir, err)
		}
	}

	// Execute each command
	for i, cmdStr := range build.Commands {
		if dryRun {
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
	if !dryRun && build.Run == "once" {
		state, err := LoadBuildState()
		if err != nil {
			return fmt.Errorf("failed to load build state: %w", err)
		}
		state.Builds[name] = BuildRecord{
			CompletedAt: time.Now(),
		}
		if err := SaveBuildState(state); err != nil {
			return fmt.Errorf("failed to save build state: %w", err)
		}
	}

	return nil
}

// RunBuilds executes all build hooks that should run
func RunBuilds(builds map[string]config.Build, dryRun bool) error {
	if len(builds) == 0 {
		return nil
	}

	fmt.Println("\nProcessing builds...")
	for name, build := range builds {
		if err := RunBuild(name, build, dryRun); err != nil {
			return fmt.Errorf("build '%s' failed: %w", name, err)
		}
	}
	return nil
}
