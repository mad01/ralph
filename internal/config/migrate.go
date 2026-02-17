package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// MigrateFromLegacy migrates from old ~/.config/dotter/ to ~/.config/ralph/
// Called at the start of ralph apply, before config loading.
func MigrateFromLegacy() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get user home directory: %w", err)
	}

	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(homeDir, ".config")
	}

	newDir := filepath.Join(xdgConfigHome, "ralph")
	oldDir := filepath.Join(xdgConfigHome, "dotter")

	// If new config dir already exists, skip (already migrated or fresh install with ralph)
	if _, err := os.Stat(newDir); err == nil {
		return nil
	}

	// If old config dir exists, rename it
	if _, err := os.Stat(oldDir); err == nil {
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("failed to migrate config directory from %s to %s: %w", oldDir, newDir, err)
		}
		fmt.Printf("Migrated configuration directory: %s -> %s\n", oldDir, newDir)
		return nil
	}

	// Neither exists = fresh install, not an error
	return nil
}
