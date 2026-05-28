package commands

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/migrate"
	"github.com/spf13/cobra"
)

var migrateStatus bool

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate symlinks after reorganizing dotfiles repository",
	Long: `Migrate updates symlinks that point to old/legacy paths after reorganizing
your dotfiles repository structure.

When you reorganize your dotfiles repo (e.g., moving files to recipe directories),
existing symlinks will point to the old paths that no longer exist. This command
detects such broken symlinks and updates them to point to the new locations.

For this to work, your recipes must define legacy_paths mappings:

  [recipe.legacy_paths]
  "ralph_files/nvim/init.lua" = "nvim/init.lua"
  "ralph_files/nvim" = "nvim"

Example workflow:
  1. Reorganize files in your dotfiles repo
  2. Create recipe.toml files with legacy_paths mappings
  3. Update config.toml to reference the recipes
  4. Run 'ralph migrate --dry-run' to preview changes
  5. Run 'ralph migrate' to update symlinks
  6. Run 'ralph up' to ensure everything is in sync
  7. Run 'ralph migrate --status' to confirm all legacy paths are gone`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}

		// --status: check whether legacy source paths still exist on disk
		if migrateStatus {
			report, err := migrate.CheckMigrationStatus(cfg)
			if err != nil {
				return fmt.Errorf("checking migration status: %w", err)
			}
			migrate.PrintMigrationStatus(os.Stdout, report)
			return nil
		}

		fmt.Println("Checking for symlinks that need migration...")

		if dryRun {
			printDryRunBanner(os.Stdout)
		}

		// Check for legacy paths in loaded recipes
		legacyPaths := config.GetAllLegacyPaths(cfg)
		if len(legacyPaths) == 0 {
			fmt.Println("\nNo legacy path mappings found in recipes.")
			fmt.Println("If you've reorganized your dotfiles, add [recipe.legacy_paths] to your recipe files.")
			fmt.Println("Example:")
			fmt.Println("  [recipe.legacy_paths]")
			fmt.Println("  \"old/path/file.txt\" = \"new/path/file.txt\"")
			return nil
		}

		fmt.Printf("Found %d legacy path mapping(s) in recipes.\n", len(legacyPaths))

		// Check migration status
		plan, err := migrate.CheckMigration(cfg)
		if err != nil {
			return fmt.Errorf("checking migration: %w", err)
		}

		// Print the plan
		migrate.PrintMigrationPlan(os.Stdout, plan)

		if plan.NeedsUpdate == 0 {
			color.Green("No symlinks need to be updated.")
			return nil
		}

		// Execute migration
		if err := migrate.ExecuteMigration(os.Stdout, plan, dryRun); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}

		fmt.Println()
		if dryRun {
			color.Cyan("DRY RUN: Migration preview complete. Run without --dry-run to apply changes.")
		} else {
			color.Green("Migration complete. Run 'ralph up' to ensure everything is in sync.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().BoolVar(&migrateStatus, "status", false, "Show which legacy_paths still exist on disk without running migration")
}
