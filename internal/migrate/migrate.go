package migrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mad01/ralph/internal/config"
)

// MigrationResult represents the result of checking a single symlink
type MigrationResult struct {
	Target        string // Target path (e.g., ~/.config/nvim/init.lua)
	CurrentSource string // Where the symlink currently points
	NewSource     string // Where the symlink should point
	Status        MigrationStatus
	Error         error
}

// MigrationStatus indicates the status of a symlink migration
type MigrationStatus int

const (
	// StatusAlreadyCorrect means the symlink already points to the correct location
	StatusAlreadyCorrect MigrationStatus = iota
	// StatusNeedsUpdate means the symlink points to a legacy path and needs updating
	StatusNeedsUpdate
	// StatusBroken means the symlink is broken but no legacy mapping was found
	StatusBroken
	// StatusNotSymlink means the target exists but is not a symlink
	StatusNotSymlink
	// StatusNotExist means the target doesn't exist (will be created by apply)
	StatusNotExist
	// StatusError means an error occurred checking the symlink
	StatusError
)

func (s MigrationStatus) String() string {
	switch s {
	case StatusAlreadyCorrect:
		return "CORRECT"
	case StatusNeedsUpdate:
		return "UPDATE"
	case StatusBroken:
		return "BROKEN"
	case StatusNotSymlink:
		return "NOT_SYMLINK"
	case StatusNotExist:
		return "NOT_EXIST"
	case StatusError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// MigrationPlan contains all migration results
type MigrationPlan struct {
	Results       []MigrationResult
	NeedsUpdate   int
	AlreadyOK     int
	Broken        int
	NotSymlinks   int
	NotExist      int
	Errors        int
	RepoPath      string
	LegacyPathMap map[string]string // old path -> new path
}

// CheckMigration analyzes all configured dotfiles and checks if their symlinks
// need to be updated due to path reorganization.
func CheckMigration(cfg *config.Config) (*MigrationPlan, error) {
	expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dotfiles repo path: %w", err)
	}

	// Get all legacy path mappings
	legacyPaths := config.GetAllLegacyPaths(cfg)

	plan := &MigrationPlan{
		RepoPath:      expandedRepoPath,
		LegacyPathMap: legacyPaths,
	}

	// Check each dotfile
	for name, df := range cfg.Dotfiles {
		result := checkSymlink(name, df, expandedRepoPath, legacyPaths)
		plan.Results = append(plan.Results, result)

		switch result.Status {
		case StatusAlreadyCorrect:
			plan.AlreadyOK++
		case StatusNeedsUpdate:
			plan.NeedsUpdate++
		case StatusBroken:
			plan.Broken++
		case StatusNotSymlink:
			plan.NotSymlinks++
		case StatusNotExist:
			plan.NotExist++
		case StatusError:
			plan.Errors++
		}
	}

	return plan, nil
}

// checkSymlink checks a single dotfile's symlink status
func checkSymlink(_ string, df config.Dotfile, repoPath string, legacyPaths map[string]string) MigrationResult {
	result := MigrationResult{}

	// Expand target path
	expandedTarget, err := config.ExpandPath(df.Target)
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Errorf("failed to expand target path: %w", err)
		return result
	}
	result.Target = expandedTarget

	// Calculate expected source path
	expectedSource := filepath.Join(repoPath, df.Source)
	result.NewSource = expectedSource

	// Check if target exists
	info, err := os.Lstat(expandedTarget)
	if os.IsNotExist(err) {
		result.Status = StatusNotExist
		return result
	}
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Errorf("failed to stat target: %w", err)
		return result
	}

	// Copy-action dotfiles are regular files by design — not symlinks.
	if df.Action == "copy" {
		result.Status = StatusAlreadyCorrect
		return result
	}

	// Check if it's a symlink
	if info.Mode()&os.ModeSymlink == 0 {
		result.Status = StatusNotSymlink
		return result
	}

	// Read the symlink target
	linkDest, err := os.Readlink(expandedTarget)
	if err != nil {
		result.Status = StatusError
		result.Error = fmt.Errorf("failed to read symlink: %w", err)
		return result
	}
	result.CurrentSource = linkDest

	// Check if symlink already points to the correct location
	if linkDest == expectedSource {
		result.Status = StatusAlreadyCorrect
		return result
	}

	// Check if symlink points to a legacy path
	for oldPath, newPath := range legacyPaths {
		oldAbsPath := filepath.Join(repoPath, oldPath)
		newAbsPath := filepath.Join(repoPath, newPath)

		// Check if current link destination matches or ends with the old path
		if linkDest == oldAbsPath || strings.HasSuffix(linkDest, "/"+oldPath) {
			// Verify the new path matches our expected source
			if newAbsPath == expectedSource || strings.HasSuffix(expectedSource, "/"+newPath) {
				result.Status = StatusNeedsUpdate
				return result
			}
		}
	}

	// Symlink points somewhere else - check if it's broken
	if _, err := os.Stat(linkDest); os.IsNotExist(err) {
		result.Status = StatusBroken
		return result
	}

	// Symlink exists and points to a valid file, but not what we expect
	// This could be intentional, so we'll mark it as already correct
	// (the user may have manually set up the symlink differently)
	result.Status = StatusAlreadyCorrect
	return result
}

// ExecuteMigration performs the actual symlink updates based on the migration plan.
// If dryRun is true, it only reports what would be done.
func ExecuteMigration(w io.Writer, plan *MigrationPlan, dryRun bool) error {
	for _, result := range plan.Results {
		if result.Status != StatusNeedsUpdate {
			continue
		}

		if dryRun {
			fmt.Fprintf(w, "[DRY RUN] Would update symlink:\n")
			fmt.Fprintf(w, "  Target:  %s\n", result.Target)
			fmt.Fprintf(w, "  From:    %s\n", result.CurrentSource)
			fmt.Fprintf(w, "  To:      %s\n", result.NewSource)
			continue
		}

		// Remove old symlink
		if err := os.Remove(result.Target); err != nil {
			return fmt.Errorf("failed to remove old symlink %s: %w", result.Target, err)
		}

		// Create new symlink
		if err := os.Symlink(result.NewSource, result.Target); err != nil {
			return fmt.Errorf("failed to create new symlink %s -> %s: %w", result.Target, result.NewSource, err)
		}

		fmt.Fprintf(w, "Updated symlink: %s\n", result.Target)
		fmt.Fprintf(w, "  From: %s\n", result.CurrentSource)
		fmt.Fprintf(w, "  To:   %s\n", result.NewSource)
	}

	return nil
}

// RecipeMigrationStatus holds the per-recipe legacy-path check results.
type RecipeMigrationStatus struct {
	// RecipeName is the human-readable name of the recipe.
	RecipeName string
	// PresentPaths are legacy source paths that still exist on disk.
	PresentPaths []string
	// MissingPaths are legacy source paths that no longer exist (already cleaned up).
	MissingPaths []string
}

// MigrationStatusReport is the full result of CheckMigrationStatus.
type MigrationStatusReport struct {
	// Recipes holds one entry per recipe that has at least one legacy_path defined.
	Recipes []RecipeMigrationStatus
	// CompleteCount is the number of recipes where all legacy paths are gone.
	CompleteCount int
	// PendingCount is the number of recipes that still have legacy paths on disk.
	PendingCount int
}

// CheckMigrationStatus inspects each loaded recipe's legacy_paths and reports
// which source paths (relative to dotfiles_repo_path) still exist on disk.
// A path counts as "present" when it exists as any kind of filesystem entry
// (regular file, directory, or symlink).
func CheckMigrationStatus(cfg *config.Config) (*MigrationStatusReport, error) {
	expandedRepoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dotfiles repo path: %w", err)
	}

	report := &MigrationStatusReport{}

	for _, info := range cfg.LoadedRecipes {
		if len(info.LegacyPaths) == 0 {
			continue
		}

		status := RecipeMigrationStatus{
			RecipeName: info.Name,
		}

		// Sort keys for deterministic output.
		oldPaths := make([]string, 0, len(info.LegacyPaths))
		for oldPath := range info.LegacyPaths {
			oldPaths = append(oldPaths, oldPath)
		}
		sort.Strings(oldPaths)

		for _, oldPath := range oldPaths {
			absPath := filepath.Join(expandedRepoPath, oldPath)
			if pathExists(absPath) {
				status.PresentPaths = append(status.PresentPaths, oldPath)
			} else {
				status.MissingPaths = append(status.MissingPaths, oldPath)
			}
		}

		report.Recipes = append(report.Recipes, status)
		if len(status.PresentPaths) == 0 {
			report.CompleteCount++
		} else {
			report.PendingCount++
		}
	}

	return report, nil
}

// pathExists returns true when path exists as any filesystem entry (file, dir, or symlink),
// including broken symlinks. It uses Lstat so symlinks are not followed.
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// PrintMigrationStatus writes a human-readable summary of a MigrationStatusReport.
func PrintMigrationStatus(w io.Writer, report *MigrationStatusReport) {
	if len(report.Recipes) == 0 {
		fmt.Fprintln(w, "No recipes have legacy_paths defined.")
		fmt.Fprintln(w, "Nothing to migrate.")
		return
	}

	total := report.CompleteCount + report.PendingCount
	fmt.Fprintf(w, "Migration status for %d recipe(s) with legacy_paths:\n\n", total)

	for _, r := range report.Recipes {
		if len(r.PresentPaths) == 0 {
			fmt.Fprintf(w, "  [complete] %s\n", r.RecipeName)
			fmt.Fprintf(w, "             Migration complete — legacy_paths block can be safely removed.\n")
		} else {
			fmt.Fprintf(w, "  [pending]  %s\n", r.RecipeName)
			fmt.Fprintf(w, "             The following legacy source paths still exist on disk:\n")
			for _, p := range r.PresentPaths {
				fmt.Fprintf(w, "               - %s\n", p)
			}
			fmt.Fprintf(w, "             Run 'ralph migrate' to update symlinks pointing to these paths.\n")
		}
		fmt.Fprintln(w)
	}

	if report.PendingCount == 0 {
		fmt.Fprintf(w, "All %d migration(s) complete — legacy_paths blocks can be safely removed from all recipes.\n", total)
	} else {
		fmt.Fprintf(w, "%d of %d recipe(s) have completed migration.\n", report.CompleteCount, total)
	}
}

// PrintMigrationPlan prints a summary of the migration plan.
func PrintMigrationPlan(w io.Writer, plan *MigrationPlan) {
	fmt.Fprintln(w, "\nMigration Plan Summary")
	fmt.Fprintln(w, "======================")
	fmt.Fprintf(w, "Already correct:  %d\n", plan.AlreadyOK)
	fmt.Fprintf(w, "Needs update:     %d\n", plan.NeedsUpdate)
	fmt.Fprintf(w, "Broken symlinks:  %d\n", plan.Broken)
	fmt.Fprintf(w, "Not symlinks:     %d\n", plan.NotSymlinks)
	fmt.Fprintf(w, "Not yet created:  %d\n", plan.NotExist)
	fmt.Fprintf(w, "Errors:           %d\n", plan.Errors)
	fmt.Fprintln(w)

	if plan.NeedsUpdate > 0 {
		fmt.Fprintln(w, "Symlinks to update:")
		for _, result := range plan.Results {
			if result.Status == StatusNeedsUpdate {
				fmt.Fprintf(w, "  %s\n", result.Target)
				fmt.Fprintf(w, "    Current: %s (BROKEN)\n", result.CurrentSource)
				fmt.Fprintf(w, "    New:     %s\n", result.NewSource)
			}
		}
		fmt.Fprintln(w)
	}

	if plan.Broken > 0 {
		fmt.Fprintln(w, "Broken symlinks (no legacy mapping found):")
		for _, result := range plan.Results {
			if result.Status == StatusBroken {
				fmt.Fprintf(w, "  %s -> %s\n", result.Target, result.CurrentSource)
			}
		}
		fmt.Fprintln(w)
	}

	if plan.NotSymlinks > 0 {
		fmt.Fprintln(w, "Files that are not symlinks (manual intervention may be needed):")
		for _, result := range plan.Results {
			if result.Status == StatusNotSymlink {
				fmt.Fprintf(w, "  %s\n", result.Target)
			}
		}
		fmt.Fprintln(w)
	}

	if plan.Errors > 0 {
		fmt.Fprintln(w, "Errors:")
		for _, result := range plan.Results {
			if result.Status == StatusError {
				fmt.Fprintf(w, "  %s: %v\n", result.Target, result.Error)
			}
		}
		fmt.Fprintln(w)
	}
}
