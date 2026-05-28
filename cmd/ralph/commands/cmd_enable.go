package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

var enableCmd = &cobra.Command{
	Use:   "enable <recipe>",
	Short: "Enable a recipe override in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		recipeName := args[0]

		configPath, cfg, err := loadConfigAndPath()
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		if err := verifyRecipeExists(cfg, recipeName); err != nil {
			return fmt.Errorf("%w", err)
		}

		if err := config.RemoveRecipeOverride(configPath, recipeName); err != nil {
			return fmt.Errorf("removing override: %w", err)
		}

		fmt.Printf("Enabled recipe '%s'. Run 'ralph up' to apply.\n", recipeName)
		return nil
	},
}

var disableCmd = &cobra.Command{
	Use:   "disable <recipe>",
	Short: "Disable a recipe override in config.toml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		recipeName := args[0]

		configPath, cfg, err := loadConfigAndPath()
		if err != nil {
			return fmt.Errorf("%w", err)
		}

		if err := verifyRecipeExists(cfg, recipeName); err != nil {
			return fmt.Errorf("%w", err)
		}

		if err := config.SetRecipeOverride(configPath, recipeName, false); err != nil {
			return fmt.Errorf("setting override: %w", err)
		}

		fmt.Printf("Disabled recipe '%s'. Run 'ralph down %s' to also clean up artifacts.\n", recipeName, recipeName)
		return nil
	},
}

// loadConfigAndPath returns both the config file path and the loaded config.
func loadConfigAndPath() (string, *config.Config, error) {
	configPath, err := config.GetDefaultConfigPath()
	if err != nil {
		return "", nil, fmt.Errorf("failed to determine config path: %w", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return "", nil, fmt.Errorf("error loading configuration: %w", err)
	}

	return configPath, cfg, nil
}

// verifyRecipeExists checks that a recipe directory with recipe.toml exists
// in the dotfiles repo.
func verifyRecipeExists(cfg *config.Config, recipeName string) error {
	repoPath, err := config.ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return fmt.Errorf("error expanding dotfiles repo path: %w", err)
	}

	recipesDir := cfg.RecipesConfig.Dir
	if recipesDir == "" {
		recipesDir = config.DefaultRecipesDir
	}
	recipeFile := filepath.Join(repoPath, recipesDir, recipeName, "recipe.toml")
	if _, err := os.Stat(recipeFile); os.IsNotExist(err) {
		return fmt.Errorf("recipe '%s' not found in %s", recipeName, filepath.Join(repoPath, "recipes"))
	}

	return nil
}

func init() {
	rootCmd.AddCommand(enableCmd)
	rootCmd.AddCommand(disableCmd)
}
