package buildstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BuildState tracks the completion status of builds with run = "once"
type BuildState struct {
	Builds map[string]BuildRecord `json:"builds"`
}

// BuildRecord holds information about a completed build
type BuildRecord struct {
	CompletedAt time.Time `json:"completed_at"`
	GitHash     string    `json:"git_hash,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
	Version     string    `json:"version,omitempty"`
	InstallHash string    `json:"install_hash,omitempty"` // sha256 of install_paths contents; drives service-restart-on-change
}

// GetStateFilePath returns the path to the builds state file.
// It is a variable to allow tests to override the path without mutating $HOME.
var GetStateFilePath = getStateFilePathInternal

func getStateFilePathInternal() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "ralph", ".builds_state"), nil
}

// LoadBuildState loads the build state from the state file
func LoadBuildState() (*BuildState, error) {
	statePath, err := GetStateFilePath()
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
	statePath, err := GetStateFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing temp build state file: %w", err)
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming build state file: %w", err)
	}

	return nil
}

// ResetBuildState clears all build state
func ResetBuildState(w io.Writer) error {
	statePath, err := GetStateFilePath()
	if err != nil {
		return err
	}

	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}

	fmt.Fprintln(w, "Build state has been reset.")
	return nil
}

// ResetBuildStateForName clears the build state for a specific build
func ResetBuildStateForName(w io.Writer, name string) error {
	state, err := LoadBuildState()
	if err != nil {
		return err
	}

	if _, exists := state.Builds[name]; exists {
		delete(state.Builds, name)
		if err := SaveBuildState(state); err != nil {
			return err
		}
		fmt.Fprintf(w, "Build state for '%s' has been reset.\n", name)
	}
	return nil
}

// ComputeBuildHash returns a stable hex-encoded sha256 over the build's
// identity (name + commands + script + working_dir).
func ComputeBuildHash(name string, commands []string, script string, workingDir string) string {
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

// ComputeInstallHash returns a stable hex-encoded sha256 over the contents of
// the given installed artifact paths. Paths must be absolute (HOME already
// expanded) — buildstate intentionally has no config dependency. Paths are
// sorted so order is irrelevant, and each path name is mixed in alongside its
// bytes so a rename is detected. An empty list yields ("", nil); a missing or
// unreadable path is an error so a misconfigured install_paths cannot silently
// masquerade as "unchanged".
func ComputeInstallHash(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, p := range sorted {
		f, err := os.Open(p)
		if err != nil {
			return "", fmt.Errorf("hashing install path %q: %w", p, err)
		}
		h.Write([]byte(p))
		h.Write([]byte{0})
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("reading install path %q: %w", p, err)
		}
		_ = f.Close()
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
