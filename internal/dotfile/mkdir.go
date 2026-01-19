package dotfile

import (
	"fmt"
	"os"
	"strconv"

	"github.com/mad01/dotter/internal/config"
)

// CreateDirectory creates a directory at the specified target path.
// If dryRun is true, it will only print the actions it would take.
func CreateDirectory(dir config.Directory, dryRun bool) error {
	absoluteTarget, err := config.ExpandPath(dir.Target)
	if err != nil {
		return fmt.Errorf("failed to expand target path '%s': %w", dir.Target, err)
	}

	// Parse mode, default to 0755
	mode := os.FileMode(0755)
	if dir.Mode != "" {
		parsed, err := strconv.ParseUint(dir.Mode, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid mode '%s': %w", dir.Mode, err)
		}
		mode = os.FileMode(parsed)
	}

	// Check if directory already exists
	info, err := os.Stat(absoluteTarget)
	if err == nil {
		if info.IsDir() {
			fmt.Printf("Directory '%s' already exists. Skipping.\n", absoluteTarget)
			return nil
		}
		return fmt.Errorf("target '%s' exists but is not a directory", absoluteTarget)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	if dryRun {
		fmt.Printf("[DRY RUN] Would create directory: '%s' (mode: %04o)\n", absoluteTarget, mode)
	} else {
		fmt.Printf("Creating directory: '%s' (mode: %04o)\n", absoluteTarget, mode)
		if err := os.MkdirAll(absoluteTarget, mode); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", absoluteTarget, err)
		}
	}

	return nil
}
