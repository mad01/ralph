package dotfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
)

// SymlinkAction defines the action to take if a file already exists at the target location.
type SymlinkAction int

const (
	// SymlinkActionBackup backups the existing file.
	SymlinkActionBackup SymlinkAction = iota
	// SymlinkActionOverwrite overwrites the existing file.
	SymlinkActionOverwrite
	// SymlinkActionSkip skips symlinking if the target exists.
	SymlinkActionSkip
)

var (
	faint = color.New(color.Faint).SprintFunc()
)

// CreateSymlink creates a symbolic link from source to target.
// It handles path expansion for both source (relative to repoPath) and target.
// If repoPath is empty, dotfileCfg.Source is assumed to be an absolute path already.
// It also manages existing files at the target location based on the specified action.
// If dryRun is true, it will only print the actions it would take.
func CreateSymlink(w io.Writer, dotfileCfg config.Dotfile, dotfilesRepoPath string, action SymlinkAction, dryRun bool) error {
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
	if !dryRun || (dryRun && dotfilesRepoPath != "") { // if it's a template dry run, source might be faked
		if _, err := os.Stat(absoluteSource); os.IsNotExist(err) {
			return fmt.Errorf("source file '%s' (expanded: '%s') does not exist", dotfileCfg.Source, absoluteSource)
		}
	}

	targetInfo, err := os.Lstat(absoluteTarget)
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
			if targetInfo.Mode()&os.ModeSymlink != 0 {
				linkTarget, readErr := os.Readlink(absoluteTarget)
				if readErr == nil && linkTarget == absoluteSource {
					fmt.Fprintf(w, "    %s\n", color.GreenString("already linked"))
					return nil
				}
			}
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
		fmt.Fprintf(w, "    %s would link\n", color.CyanString("[dry run]"))
	} else {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", targetDir, err)
		}
		fmt.Fprintf(w, "    %s\n", color.GreenString("linked"))
		if err := os.Symlink(absoluteSource, absoluteTarget); err != nil {
			return fmt.Errorf("failed to create symlink from '%s' to '%s': %w", absoluteSource, absoluteTarget, err)
		}
	}

	return nil
}

// CreateDirSymlink creates a symbolic link to a directory.
// This is equivalent to `ln -sfn` behavior - it handles existing directories
// and symlinks appropriately.
// If repoPath is empty, dotfileCfg.Source is assumed to be an absolute path.
// If dryRun is true, it will only print the actions it would take.
func CreateDirSymlink(w io.Writer, dotfileCfg config.Dotfile, dotfilesRepoPath string, action SymlinkAction, dryRun bool) error {
	var absoluteSource string
	var err error

	if dotfilesRepoPath == "" {
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

	// Ensure the source directory exists
	if !dryRun || (dryRun && dotfilesRepoPath != "") {
		info, err := os.Stat(absoluteSource)
		if os.IsNotExist(err) {
			return fmt.Errorf("source directory '%s' (expanded: '%s') does not exist", dotfileCfg.Source, absoluteSource)
		}
		if err != nil {
			return fmt.Errorf("failed to stat source '%s': %w", absoluteSource, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("source '%s' is not a directory (use regular symlink for files)", absoluteSource)
		}
	}

	// Check if target already exists (using Lstat to not follow symlinks)
	targetInfo, err := os.Lstat(absoluteTarget)
	if err == nil {
		// Target exists - check what it is
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - check if it points to our source
			linkTarget, readErr := os.Readlink(absoluteTarget)
			if readErr == nil && linkTarget == absoluteSource {
				fmt.Fprintf(w, "    %s\n", color.GreenString("already linked"))
				return nil
			}
			// It's a symlink but points elsewhere
			if err := handleExistingTarget(w, absoluteTarget, action, dryRun); err != nil {
				return err
			}
			if action == SymlinkActionSkip {
				return nil
			}
		} else if targetInfo.IsDir() {
			// It's an actual directory
			if err := handleExistingDirTarget(w, absoluteTarget, action, dryRun); err != nil {
				return err
			}
			if action == SymlinkActionSkip {
				return nil
			}
		} else {
			// It's a file
			if err := handleExistingTarget(w, absoluteTarget, action, dryRun); err != nil {
				return err
			}
			if action == SymlinkActionSkip {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	// Ensure parent directory exists
	targetDir := filepath.Dir(absoluteTarget)
	if dryRun {
		fmt.Fprintf(w, "    %s would link directory\n", color.CyanString("[dry run]"))
	} else {
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", targetDir, err)
		}
		fmt.Fprintf(w, "    %s\n", color.GreenString("linked"))
		if err := os.Symlink(absoluteSource, absoluteTarget); err != nil {
			return fmt.Errorf("failed to create symlink from '%s' to '%s': %w", absoluteSource, absoluteTarget, err)
		}
	}

	return nil
}

// handleExistingTarget handles an existing file or symlink at the target location.
func handleExistingTarget(w io.Writer, absoluteTarget string, action SymlinkAction, dryRun bool) error {
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
				return fmt.Errorf("failed to remove existing target '%s': %w", absoluteTarget, err)
			}
		}
	case SymlinkActionSkip:
		fmt.Fprintf(w, "    %s %s\n", color.CyanString("skipped"), faint("target exists"))
	}
	return nil
}

// handleExistingDirTarget handles an existing directory at the target location.
func handleExistingDirTarget(w io.Writer, absoluteTarget string, action SymlinkAction, dryRun bool) error {
	switch action {
	case SymlinkActionBackup:
		backupPath := absoluteTarget + ".bak"
		if dryRun {
			fmt.Fprintf(w, "    %s would back up directory %s %s\n", color.CyanString("[dry run]"), faint("→"), faint(config.ShortenHome(backupPath)))
		} else {
			fmt.Fprintf(w, "    %s %s %s\n", color.YellowString("backed up directory"), faint("→"), faint(config.ShortenHome(backupPath)))
			if err := os.Rename(absoluteTarget, backupPath); err != nil {
				return fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
			}
		}
	case SymlinkActionOverwrite:
		if dryRun {
			fmt.Fprintf(w, "    %s would overwrite existing directory\n", color.CyanString("[dry run]"))
		} else {
			fmt.Fprintf(w, "    %s\n", color.YellowString("overwriting existing directory"))
			if err := os.RemoveAll(absoluteTarget); err != nil {
				return fmt.Errorf("failed to remove existing directory '%s': %w", absoluteTarget, err)
			}
		}
	case SymlinkActionSkip:
		fmt.Fprintf(w, "    %s %s\n", color.CyanString("skipped"), faint("directory exists"))
	}
	return nil
}
