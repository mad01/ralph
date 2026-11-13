// Package paths resolves where ralph keeps its config file and its state
// directory. The state directory holds the state files, the run lock, the
// generated shell scripts and the default package and recipe source caches.
// Every one of those paths is built from StateDir, so a run against a scratch
// config (RALPH_CONFIG) never reads or writes the state of the real one.
package paths

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// ConfigEnv names the environment variable that points ralph at a config
	// file.
	ConfigEnv = "RALPH_CONFIG"

	// StateDirEnv names the environment variable that sets the state directory
	// outright, whatever config file is in use.
	StateDirEnv = "RALPH_STATE_DIR"

	// ConfigFileName is the basename of the config file in the default location.
	ConfigFileName = "config.toml"

	// RecipeStateFile is the basename of the recipe artifact manifest in the
	// state directory.
	RecipeStateFile = ".recipe_state"
)

// legacyWarning makes the legacy state warning print at most once per process,
// however many state paths a run resolves.
var legacyWarning sync.Once

// Expand expands a leading ~ to the user's home directory, then environment
// variables.
func Expand(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}
	return os.ExpandEnv(path), nil
}

// ConfigFile returns the path of the config file ralph uses: $RALPH_CONFIG
// when set, else $XDG_CONFIG_HOME/ralph/config.toml, else
// ~/.config/ralph/config.toml. It does not check that the file exists.
func ConfigFile() (string, error) {
	if env := os.Getenv(ConfigEnv); env != "" {
		p, err := Expand(env)
		if err != nil {
			return "", fmt.Errorf("%s=%s: %w", ConfigEnv, env, err)
		}
		return p, nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "ralph", ConfigFileName), nil
}

// StateDir returns ralph's state directory as an absolute path:
// $RALPH_STATE_DIR when set, else the directory that holds the config file
// (see ConfigFile).
//
// Before RALPH_STATE_DIR existed the state always lived in ~/.config/ralph.
// The first call in a process prints one warning on stderr when the directory
// it picks has no recipe state yet but ~/.config/ralph has one, so a user whose
// config lives elsewhere learns that the state moved and how to keep it.
func StateDir() (string, error) {
	if env := os.Getenv(StateDirEnv); env != "" {
		dir, err := absDir(env)
		if err != nil {
			return "", fmt.Errorf("%s=%s: %w", StateDirEnv, env, err)
		}
		return dir, nil
	}
	configFile, err := ConfigFile()
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(filepath.Dir(configFile))
	if err != nil {
		return "", fmt.Errorf("resolving state directory: %w", err)
	}
	legacyWarning.Do(func() { warnLegacyState(os.Stderr, dir) })
	return dir, nil
}

// absDir expands dir and makes it absolute.
func absDir(dir string) (string, error) {
	expanded, err := Expand(dir)
	if err != nil {
		return "", err
	}
	return filepath.Abs(expanded)
}

// warnLegacyState writes the state-moved warning to w when dir has no recipe
// state and the legacy directory ~/.config/ralph has one. When dir is the
// legacy directory both checks cannot hold, so the default user never sees it.
func warnLegacyState(w io.Writer, dir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	legacy := filepath.Join(home, ".config", "ralph")
	if fileExists(filepath.Join(dir, RecipeStateFile)) ||
		!fileExists(filepath.Join(legacy, RecipeStateFile)) {
		return
	}
	fmt.Fprintf(w,
		"Warning: ralph keeps its state next to the config file, in %s, but the state "+
			"from earlier runs is in %s. This run starts with empty state in %s, so builds "+
			"rerun and cleanup finds nothing to remove. To keep the old state, set %s=%s, "+
			"or move .recipe_state and .builds_state (and pkg/ and sources/) from %s to %s.\n",
		dir, legacy, dir, StateDirEnv, legacy, legacy, dir,
	)
}

// fileExists reports whether path names an existing file or directory.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
