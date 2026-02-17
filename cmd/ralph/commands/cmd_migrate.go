package commands

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/mad01/ralph/internal/migrate"
	"github.com/spf13/cobra"
)

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
  6. Run 'ralph apply' to ensure everything is in sync`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Checking for symlinks that need migration...")

		if dryRun {
			color.Cyan("\n*** DRY RUN MODE ENABLED ***")
			color.Cyan("No actual changes will be made.")
			color.Cyan("****************************\n")
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error loading configuration: %v", err))
			os.Exit(1)
		}

		// Check for legacy paths in loaded recipes
		legacyPaths := config.GetAllLegacyPaths(cfg)
		if len(legacyPaths) == 0 {
			fmt.Println("\nNo legacy path mappings found in recipes.")
			fmt.Println("If you've reorganized your dotfiles, add [recipe.legacy_paths] to your recipe files.")
			fmt.Println("Example:")
			fmt.Println("  [recipe.legacy_paths]")
			fmt.Println("  \"old/path/file.txt\" = \"new/path/file.txt\"")
			return
		}

		fmt.Printf("Found %d legacy path mapping(s) in recipes.\n", len(legacyPaths))

		// Check migration status
		plan, err := migrate.CheckMigration(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error checking migration: %v", err))
			os.Exit(1)
		}

		// Print the plan
		migrate.PrintMigrationPlan(plan)

		if plan.NeedsUpdate == 0 {
			color.Green("No symlinks need to be updated.")
			return
		}

		// Execute migration
		if err := migrate.ExecuteMigration(plan, dryRun); err != nil {
			fmt.Fprintln(os.Stderr, color.RedString("Error executing migration: %v", err))
			os.Exit(1)
		}

		fmt.Println()
		if dryRun {
			color.Cyan("DRY RUN: Migration preview complete. Run without --dry-run to apply changes.")
		} else {
			color.Green("Migration complete. Run 'ralph apply' to ensure everything is in sync.")
		}
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
