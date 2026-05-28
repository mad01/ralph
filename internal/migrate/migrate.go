package migrate

import (
	"fmt"
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
func ExecuteMigration(plan *MigrationPlan, dryRun bool) error {
	for _, result := range plan.Results {
		if result.Status != StatusNeedsUpdate {
			continue
		}

		if dryRun {
			fmt.Printf("[DRY RUN] Would update symlink:\n")
			fmt.Printf("  Target:  %s\n", result.Target)
			fmt.Printf("  From:    %s\n", result.CurrentSource)
			fmt.Printf("  To:      %s\n", result.NewSource)
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

		fmt.Printf("Updated symlink: %s\n", result.Target)
		fmt.Printf("  From: %s\n", result.CurrentSource)
		fmt.Printf("  To:   %s\n", result.NewSource)
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

// PrintMigrationStatus writes a human-readable summary of a MigrationStatusReport
// to stdout using the provided color helpers.
func PrintMigrationStatus(report *MigrationStatusReport) {
	if len(report.Recipes) == 0 {
		fmt.Println("No recipes have legacy_paths defined.")
		fmt.Println("Nothing to migrate.")
		return
	}

	total := report.CompleteCount + report.PendingCount
	fmt.Printf("Migration status for %d recipe(s) with legacy_paths:\n\n", total)

	for _, r := range report.Recipes {
		if len(r.PresentPaths) == 0 {
			fmt.Printf("  [complete] %s\n", r.RecipeName)
			fmt.Printf("             Migration complete — legacy_paths block can be safely removed.\n")
		} else {
			fmt.Printf("  [pending]  %s\n", r.RecipeName)
			fmt.Printf("             The following legacy source paths still exist on disk:\n")
			for _, p := range r.PresentPaths {
				fmt.Printf("               - %s\n", p)
			}
			fmt.Printf("             Run 'ralph migrate' to update symlinks pointing to these paths.\n")
		}
		fmt.Println()
	}

	if report.PendingCount == 0 {
		fmt.Printf("All %d migration(s) complete — legacy_paths blocks can be safely removed from all recipes.\n", total)
	} else {
		fmt.Printf("%d of %d recipe(s) have completed migration.\n", report.CompleteCount, total)
	}
}

// PrintMigrationPlan prints a summary of the migration plan
func PrintMigrationPlan(plan *MigrationPlan) {
	fmt.Println("\nMigration Plan Summary")
	fmt.Println("======================")
	fmt.Printf("Already correct:  %d\n", plan.AlreadyOK)
	fmt.Printf("Needs update:     %d\n", plan.NeedsUpdate)
	fmt.Printf("Broken symlinks:  %d\n", plan.Broken)
	fmt.Printf("Not symlinks:     %d\n", plan.NotSymlinks)
	fmt.Printf("Not yet created:  %d\n", plan.NotExist)
	fmt.Printf("Errors:           %d\n", plan.Errors)
	fmt.Println()

	if plan.NeedsUpdate > 0 {
		fmt.Println("Symlinks to update:")
		for _, result := range plan.Results {
			if result.Status == StatusNeedsUpdate {
				fmt.Printf("  %s\n", result.Target)
				fmt.Printf("    Current: %s (BROKEN)\n", result.CurrentSource)
				fmt.Printf("    New:     %s\n", result.NewSource)
			}
		}
		fmt.Println()
	}

	if plan.Broken > 0 {
		fmt.Println("Broken symlinks (no legacy mapping found):")
		for _, result := range plan.Results {
			if result.Status == StatusBroken {
				fmt.Printf("  %s -> %s\n", result.Target, result.CurrentSource)
			}
		}
		fmt.Println()
	}

	if plan.NotSymlinks > 0 {
		fmt.Println("Files that are not symlinks (manual intervention may be needed):")
		for _, result := range plan.Results {
			if result.Status == StatusNotSymlink {
				fmt.Printf("  %s\n", result.Target)
			}
		}
		fmt.Println()
	}

	if plan.Errors > 0 {
		fmt.Println("Errors:")
		for _, result := range plan.Results {
			if result.Status == StatusError {
				fmt.Printf("  %s: %v\n", result.Target, result.Error)
			}
		}
		fmt.Println()
	}
}
