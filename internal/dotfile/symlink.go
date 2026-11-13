package dotfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

	// ErrSkipped is returned when an operation is skipped because the target
	// already exists and the action is SymlinkActionSkip.
	ErrSkipped = fmt.Errorf("skipped: target exists")

	// osSymlink is os.Symlink, held in a variable so tests can make linking
	// fail after an existing target was moved to its backup.
	osSymlink = os.Symlink
)

// backupTimestampLayout is the time format makeBackupPath appends after
// ".bak.". backupTimestampPattern matches exactly that format so cleanup only
// touches backups ralph itself created.
const backupTimestampLayout = "20060102T150405.000000000"

var backupTimestampPattern = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}$`)

func makeBackupPath(target string) string {
	return target + ".bak." + time.Now().Format(backupTimestampLayout)
}

// cleanupStaleBackups reaps the redundant backups older ralph versions left
// when they backed up an already-correct link on every apply. It removes a
// sibling of absoluteTarget only when its name matches ralph's exact backup
// format (<base>.bak.<timestamp>) and it is a symlink to absoluteSource, so
// it duplicates the link ralph just confirmed and holds nothing else.
//
// Everything else is kept: regular-file and directory backups (user data
// ralph replaced), names that only resemble a backup (a foreign or
// attacker-planted "<base>.bak.evil"), and backup symlinks pointing anywhere
// else. Those last ones are the user's own links ralph moved aside, which
// includes the backup made in the current call. Removal failures are
// reported and not counted as removed.
func cleanupStaleBackups(w io.Writer, absoluteTarget, absoluteSource string, dryRun bool) int {
	dir := filepath.Dir(absoluteTarget)
	base := filepath.Base(absoluteTarget)
	prefix := base + ".bak."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !backupTimestampPattern.MatchString(name[len(prefix):]) {
			continue
		}
		full := filepath.Join(dir, name)
		// Readlink fails on anything that is not a symlink.
		if dest, err := os.Readlink(full); err != nil || dest != absoluteSource {
			continue
		}
		if dryRun {
			fmt.Fprintf(
				w,
				"    %s would remove stale backup %s\n",
				color.CyanString("[dry run]"),
				faint(config.ShortenHome(full)),
			)
		} else if err := os.Remove(full); err != nil {
			fmt.Fprintf(w, "    %s could not remove stale backup %s: %v\n", color.YellowString("warning"), faint(config.ShortenHome(full)), err)
			continue
		}
		removed++
	}
	if removed > 0 && !dryRun {
		fmt.Fprintf(w, "    %s %d stale backup(s)\n", color.YellowString("cleaned"), removed)
	}
	return removed
}

// CreateSymlink creates a symbolic link from source to target.
// It handles path expansion for both source (relative to repoPath) and target.
// If repoPath is empty, dotfileCfg.Source is assumed to be an absolute path already.
// It also manages existing files at the target location based on the specified action.
// If dryRun is true, it will only print the actions it would take.
func CreateSymlink(
	w io.Writer,
	dotfileCfg config.Dotfile,
	dotfilesRepoPath string,
	action SymlinkAction,
	dryRun bool,
) error {
	var absoluteSource string
	var err error

	if dotfilesRepoPath == "" { // Source is already absolute (e.g., a processed template file)
		absoluteSource = dotfileCfg.Source
	} else {
		absoluteSource, err = config.ExpandPath(config.JoinSourcePath(dotfilesRepoPath, dotfileCfg.Source))
		if err != nil {
			return fmt.Errorf("failed to expand source path '%s' relative to '%s': %w", dotfileCfg.Source, dotfilesRepoPath, err)
		}
	}

	absoluteTarget, err := config.ExpandPath(dotfileCfg.Target)
	if err != nil {
		return fmt.Errorf("failed to expand target path '%s': %w", dotfileCfg.Target, err)
	}

	// Ensure the source file exists (unless it's a dry run for a templated file that wouldn't exist yet)
	if !dryRun ||
		(dryRun && dotfilesRepoPath != "") { // if it's a template dry run, source might be faked
		if _, err := os.Stat(absoluteSource); os.IsNotExist(err) {
			return fmt.Errorf(
				"source file '%s' (expanded: '%s') does not exist",
				dotfileCfg.Source,
				absoluteSource,
			)
		}
	}

	// backupPath is where the existing target was moved, if it was.
	var backupPath string
	targetInfo, err := os.Lstat(absoluteTarget)
	if err == nil {
		// Short-circuit: if target is already a symlink pointing to the correct source, skip.
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			linkTarget, readErr := os.Readlink(absoluteTarget)
			if readErr == nil && linkTarget == absoluteSource {
				fmt.Fprintf(w, "    %s\n", color.GreenString("already linked"))
				cleanupStaleBackups(w, absoluteTarget, absoluteSource, dryRun)
				return nil
			}
		}

		switch action {
		case SymlinkActionBackup:
			backupPath = makeBackupPath(absoluteTarget)
			if dryRun {
				fmt.Fprintf(
					w,
					"    %s would back up %s %s\n",
					color.CyanString("[dry run]"),
					faint("→"),
					faint(config.ShortenHome(backupPath)),
				)
			} else {
				fmt.Fprintf(w, "    %s %s %s\n", color.YellowString("backed up"), faint("→"), faint(config.ShortenHome(backupPath)))
				if err := os.Rename(absoluteTarget, backupPath); err != nil {
					return fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
				}
			}
		case SymlinkActionOverwrite:
			if dryRun {
				fmt.Fprintf(
					w,
					"    %s would overwrite existing\n",
					color.CyanString("[dry run]"),
				)
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
			fmt.Fprintf(
				w,
				"    %s %s\n",
				color.CyanString("skipped"),
				faint("target exists"),
			)
			return ErrSkipped
		default:
			return fmt.Errorf("unknown action for existing target '%s'", absoluteTarget)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	if dryRun {
		fmt.Fprintf(w, "    %s would link\n", color.CyanString("[dry run]"))
		cleanupStaleBackups(w, absoluteTarget, absoluteSource, true)
		return nil
	}
	if err := linkInPlace(absoluteSource, absoluteTarget, backupPath); err != nil {
		return err
	}
	fmt.Fprintf(w, "    %s\n", color.GreenString("linked"))
	cleanupStaleBackups(w, absoluteTarget, absoluteSource, false)
	return nil
}

// linkInPlace creates the symlink at absoluteTarget, making its parent
// directory first. backupPath is where the existing target was moved, or
// empty when nothing was moved. When linking fails, the original is moved
// back from backupPath so a failed link never leaves the target missing.
func linkInPlace(absoluteSource, absoluteTarget, backupPath string) error {
	err := createLink(absoluteSource, absoluteTarget)
	if err == nil || backupPath == "" {
		return err
	}
	if restoreErr := os.Rename(backupPath, absoluteTarget); restoreErr != nil {
		return errors.Join(
			err,
			fmt.Errorf(
				"failed to restore '%s' from backup '%s': %w",
				absoluteTarget,
				backupPath,
				restoreErr,
			),
		)
	}
	return err
}

func createLink(absoluteSource, absoluteTarget string) error {
	targetDir := filepath.Dir(absoluteTarget)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory '%s': %w", targetDir, err)
	}
	if err := osSymlink(absoluteSource, absoluteTarget); err != nil {
		return fmt.Errorf(
			"failed to create symlink from '%s' to '%s': %w",
			absoluteSource,
			absoluteTarget,
			err,
		)
	}
	return nil
}

// CreateDirSymlink creates a symbolic link to a directory.
// This is equivalent to `ln -sfn` behavior - it handles existing directories
// and symlinks appropriately.
// If repoPath is empty, dotfileCfg.Source is assumed to be an absolute path.
// If dryRun is true, it will only print the actions it would take.
func CreateDirSymlink(
	w io.Writer,
	dotfileCfg config.Dotfile,
	dotfilesRepoPath string,
	action SymlinkAction,
	dryRun bool,
) error {
	var absoluteSource string
	var err error

	if dotfilesRepoPath == "" {
		absoluteSource = dotfileCfg.Source
	} else {
		absoluteSource, err = config.ExpandPath(config.JoinSourcePath(dotfilesRepoPath, dotfileCfg.Source))
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
			return fmt.Errorf(
				"source directory '%s' (expanded: '%s') does not exist",
				dotfileCfg.Source,
				absoluteSource,
			)
		}
		if err != nil {
			return fmt.Errorf("failed to stat source '%s': %w", absoluteSource, err)
		}
		if !info.IsDir() {
			return fmt.Errorf(
				"source '%s' is not a directory (use regular symlink for files)",
				absoluteSource,
			)
		}
	}

	// backupPath is where the existing target was moved, if it was.
	var backupPath string
	// Check if target already exists (using Lstat to not follow symlinks)
	targetInfo, err := os.Lstat(absoluteTarget)
	if err == nil {
		// Target exists - check what it is
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - check if it points to our source
			linkTarget, readErr := os.Readlink(absoluteTarget)
			if readErr == nil && linkTarget == absoluteSource {
				fmt.Fprintf(w, "    %s\n", color.GreenString("already linked"))
				cleanupStaleBackups(w, absoluteTarget, absoluteSource, dryRun)
				return nil
			}
			// It's a symlink but points elsewhere
			backupPath, err = handleExistingTarget(w, absoluteTarget, action, dryRun)
		} else if targetInfo.IsDir() {
			// It's an actual directory
			backupPath, err = handleExistingDirTarget(w, absoluteTarget, action, dryRun)
		} else {
			// It's a file
			backupPath, err = handleExistingTarget(w, absoluteTarget, action, dryRun)
		}
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat target '%s': %w", absoluteTarget, err)
	}

	if dryRun {
		fmt.Fprintf(w, "    %s would link directory\n", color.CyanString("[dry run]"))
		cleanupStaleBackups(w, absoluteTarget, absoluteSource, true)
		return nil
	}
	if err := linkInPlace(absoluteSource, absoluteTarget, backupPath); err != nil {
		return err
	}
	fmt.Fprintf(w, "    %s\n", color.GreenString("linked"))
	cleanupStaleBackups(w, absoluteTarget, absoluteSource, false)
	return nil
}

// handleExistingTarget handles an existing file or symlink at the target
// location. It returns the path the target was moved to, or "" when nothing
// was moved.
func handleExistingTarget(
	w io.Writer,
	absoluteTarget string,
	action SymlinkAction,
	dryRun bool,
) (string, error) {
	switch action {
	case SymlinkActionBackup:
		backupPath := makeBackupPath(absoluteTarget)
		if dryRun {
			fmt.Fprintf(
				w,
				"    %s would back up %s %s\n",
				color.CyanString("[dry run]"),
				faint("→"),
				faint(config.ShortenHome(backupPath)),
			)
		} else {
			fmt.Fprintf(w, "    %s %s %s\n", color.YellowString("backed up"), faint("→"), faint(config.ShortenHome(backupPath)))
			if err := os.Rename(absoluteTarget, backupPath); err != nil {
				return "", fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
			}
			return backupPath, nil
		}
	case SymlinkActionOverwrite:
		if dryRun {
			fmt.Fprintf(
				w,
				"    %s would overwrite existing\n",
				color.CyanString("[dry run]"),
			)
		} else {
			fmt.Fprintf(w, "    %s\n", color.YellowString("overwriting existing"))
			if err := os.Remove(absoluteTarget); err != nil {
				return "", fmt.Errorf("failed to remove existing target '%s': %w", absoluteTarget, err)
			}
		}
	case SymlinkActionSkip:
		fmt.Fprintf(w, "    %s %s\n", color.CyanString("skipped"), faint("target exists"))
		return "", ErrSkipped
	}
	return "", nil
}

// handleExistingDirTarget handles an existing directory at the target
// location. It returns the path the directory was moved to, or "" when
// nothing was moved.
func handleExistingDirTarget(
	w io.Writer,
	absoluteTarget string,
	action SymlinkAction,
	dryRun bool,
) (string, error) {
	switch action {
	case SymlinkActionBackup:
		backupPath := makeBackupPath(absoluteTarget)
		if dryRun {
			fmt.Fprintf(
				w,
				"    %s would back up directory %s %s\n",
				color.CyanString("[dry run]"),
				faint("→"),
				faint(config.ShortenHome(backupPath)),
			)
		} else {
			fmt.Fprintf(w, "    %s %s %s\n", color.YellowString("backed up directory"), faint("→"), faint(config.ShortenHome(backupPath)))
			if err := os.Rename(absoluteTarget, backupPath); err != nil {
				return "", fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
			}
			return backupPath, nil
		}
	case SymlinkActionOverwrite:
		entries, err := os.ReadDir(absoluteTarget)
		if err != nil {
			return "", fmt.Errorf("failed to read directory '%s': %w", absoluteTarget, err)
		}
		if len(entries) > 0 {
			// Non-empty directory: back up instead of destroying to prevent data loss
			backupPath := makeBackupPath(absoluteTarget)
			if dryRun {
				fmt.Fprintf(
					w,
					"    %s would back up non-empty directory (%d items) %s %s\n",
					color.CyanString(
						"[dry run]",
					),
					len(entries),
					faint("→"),
					faint(config.ShortenHome(backupPath)),
				)
			} else {
				fmt.Fprintf(w, "    %s non-empty directory (%d items) %s %s\n",
					color.YellowString("backed up"), len(entries), faint("→"), faint(config.ShortenHome(backupPath)))
				if err := os.Rename(absoluteTarget, backupPath); err != nil {
					return "", fmt.Errorf("failed to backup '%s' to '%s': %w", absoluteTarget, backupPath, err)
				}
				return backupPath, nil
			}
		} else {
			if dryRun {
				fmt.Fprintf(w, "    %s would overwrite empty directory\n", color.CyanString("[dry run]"))
			} else {
				fmt.Fprintf(w, "    %s\n", color.YellowString("overwriting empty directory"))
				if err := os.RemoveAll(absoluteTarget); err != nil {
					return "", fmt.Errorf("failed to remove existing directory '%s': %w", absoluteTarget, err)
				}
			}
		}
	case SymlinkActionSkip:
		fmt.Fprintf(w, "    %s %s\n", color.CyanString("skipped"), faint("directory exists"))
		return "", ErrSkipped
	}
	return "", nil
}
