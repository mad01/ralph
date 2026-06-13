package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/fatih/color"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

var addDescription string

var validRecipeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

var addCmd = &cobra.Command{
	Use:   "add <recipe-name>",
	Short: "Create a new recipe scaffold in the dotfiles repo",
	Long:  `Creates a new recipe directory with a template recipe.toml under <dotfiles_repo_path>/recipes/<name>/.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		if !validRecipeName.MatchString(name) {
			return fmt.Errorf(
				"invalid recipe name '%s': must be alphanumeric with hyphens only (no leading hyphen)",
				name,
			)
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		repoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
		if err != nil {
			return fmt.Errorf("failed to expand dotfiles_repo_path: %w", err)
		}

		recipeDir := filepath.Join(repoPath, "recipes", name)

		if _, err := os.Stat(recipeDir); err == nil {
			return fmt.Errorf("recipe '%s' already exists at %s", name, recipeDir)
		}

		if dryRun {
			fmt.Printf("Would create recipe directory: %s\n", recipeDir)
			fmt.Printf("Would write recipe.toml template\n")
			return nil
		}

		if err := os.MkdirAll(recipeDir, 0o755); err != nil {
			return fmt.Errorf("failed to create recipe directory: %w", err)
		}

		tomlContent := fmt.Sprintf(`[recipe]
name = %q
description = %q
delete_behavior = "delete"

# Dotfiles — symlink config files into place
# [dotfiles.example_config]
# source = "config"
# target = "~/.config/%s/config"

# Shell aliases
# [shell.aliases.example]
# command = "echo hello"

# Shell functions
# [shell.functions.example]
# body = "echo hello $1"

# Build hooks
# [hooks.builds.example]
# commands = ["make build"]
# working_dir = "~/code/example"
# run = "once"
`, name, addDescription, name)

		recipePath := filepath.Join(recipeDir, "recipe.toml")
		if err := os.WriteFile(recipePath, []byte(tomlContent), 0o644); err != nil {
			return fmt.Errorf("failed to write recipe.toml: %w", err)
		}

		fmt.Println(color.GreenString("Created recipe '%s' at %s", name, recipeDir))
		fmt.Println("\nNext steps:")
		fmt.Printf("  1. Edit %s\n", config.ShortenHome(recipePath))
		fmt.Println("  2. Run 'ralph up' to apply the new recipe")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(&addDescription, "description", "", "Set the recipe description")
}
