package dotfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
)

// CopyFile copies a dotfile from source to target.
// It handles path expansion for both source (relative to repoPath) and target.
// If repoPath is empty, dotfileCfg.Source is assumed to be an absolute path already.
// It also manages existing files at the target location based on the specified action.
// If dryRun is true, it will only print the actions it would take.
func CopyFile(w io.Writer, dotfileCfg config.Dotfile, dotfilesRepoPath string, action SymlinkAction, dryRun bool) error {
	var absoluteSource string
	var err error

	if dotfilesRepoPath == "" { // Source is already absolute (e.g., a processed template file)
		absoluteSource = dotfileCfg.Source
	} else {
		absoluteSource, err = config.ExpandPath(filepath.Join(dotfilesRepoPath, dotfileCfg.Source))
		if err != nil {
			return fmt.Errorf("failed to expand source path '%s' relative to '%s': %w", dotfileCfg.Source, dotfilesRepoPath, err)
		}
	}

	absoluteTarget, err := config.ExpandPath(dotfileCfg.Target)
	if err != nil {
		return fmt.Errorf("failed to expand target path '%s': %w", dotfileCfg.Target, err)
	}

	// Ensure the source file exists (unless it's a dry run for a templated file that wouldn't exist yet)
	if !dryRun || (dryRun && dotfilesRepoPath != "") {
		if _, err := os.Stat(absoluteSource); os.IsNotExist(err) {
			return fmt.Errorf("source file '%s' (expanded: '%s') does not exist", dotfileCfg.Source, absoluteSource)
		}
	}

	// Handle existing target file
	_, err = os.Lstat(absoluteTarget)
	if err == nil {
		switch action {
		case SymlinkActionBackup:
			backupPath := absoluteTarget + ".bak"
			if dryRun {
				fmt.Fprintf(w, "    %s would back up %s %s\n", color.CyanString("[dry run]"), faint("→"), faint(config.ShortenHome(backupPath)))
			} else {
				fmt.Fprintf(w, "    %s %s %s\n", color.YellowString("backed up"), faint("→"), faint(config.ShortenHome(backupPath)))
				if err := os.Rename(absoluteTarget, backupPath); err != nil {
					return fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
				}
			}
		case SymlinkActionOverwrite:
			if dryRun {
				fmt.Fprintf(w, "    %s would overwrite existing\n", color.CyanString("[dry run]"))
			} else {
				fmt.Fprintf(w, "    %s\n", color.YellowString("overwriting existing"))
				if err := os.Remove(absoluteTarget); err != nil {
					return fmt.Errorf("failed to remove existing target '%s' for overwrite: %w", absoluteTarget, err)
				}
			}
		case SymlinkActionSkip:
			fmt.Fprintf(w, "    %s %s\n", color.CyanString("skipped"), faint("target exists"))
			return nil
		default:
			return fmt.Errorf("unknown action for existing target '%s'", absoluteTarget)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	targetDir := filepath.Dir(absoluteTarget)
	if dryRun {
		fmt.Fprintf(w, "    %s would copy\n", color.CyanString("[dry run]"))
	} else {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", targetDir, err)
		}
		fmt.Fprintf(w, "    %s\n", color.GreenString("copied"))
		if err := copyFileContents(absoluteSource, absoluteTarget); err != nil {
			return fmt.Errorf("failed to copy file from '%s' to '%s': %w", absoluteSource, absoluteTarget, err)
		}
	}

	return nil
}

// copyFileContents copies the contents of the source file to the target file,
// preserving the source file's permissions.
func copyFileContents(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
