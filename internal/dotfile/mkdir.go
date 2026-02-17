package dotfile

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
)

// CreateDirectory creates a directory at the specified target path.
// If dryRun is true, it will only print the actions it would take.
func CreateDirectory(w io.Writer, dir config.Directory, dryRun bool) error {
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
			fmt.Fprintf(w, "    %s\n", color.GreenString("already exists"))
			return nil
		}
		return fmt.Errorf("target '%s' exists but is not a directory", absoluteTarget)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	if dryRun {
		fmt.Fprintf(w, "    %s would create %s\n", color.CyanString("[dry run]"), faint(fmt.Sprintf("mode %04o", mode)))
	} else {
		fmt.Fprintf(w, "    %s %s\n", color.GreenString("created"), faint(fmt.Sprintf("mode %04o", mode)))
		if err := os.MkdirAll(absoluteTarget, mode); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", absoluteTarget, err)
		}
	}

	return nil
}
