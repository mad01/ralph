package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

		fmt.Printf(
			"Disabled recipe '%s'. Run 'ralph up --enable-cleanup' to remove its artifacts.\n",
			recipeName,
		)
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
// in the dotfiles repo, or — for a namespaced "<source>/<recipe>" name — in
// the recipe source's cached checkout.
func verifyRecipeExists(cfg *config.Config, recipeName string) error {
	if source, recipe, ok := strings.Cut(recipeName, "/"); ok {
		return verifySourceRecipeExists(cfg, source, recipe)
	}

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
		return fmt.Errorf(
			"recipe '%s' not found in %s",
			recipeName,
			filepath.Join(repoPath, "recipes"),
		)
	}

	return nil
}

// verifySourceRecipeExists checks that a recipe exists in the named recipe
// source's cached checkout.
func verifySourceRecipeExists(cfg *config.Config, sourceName, recipeName string) error {
	for _, src := range cfg.RecipeSources {
		if src.Name != sourceName {
			continue
		}
		sourcesDir, err := config.SourcesDir()
		if err != nil {
			return fmt.Errorf("error expanding sources dir: %w", err)
		}
		checkout := config.SourceCheckoutPath(sourcesDir, src)
		recipeFile := filepath.Join(
			checkout,
			config.SourceRecipesDir(src),
			recipeName,
			"recipe.toml",
		)
		if _, err := os.Stat(recipeFile); os.IsNotExist(err) {
			return fmt.Errorf(
				"recipe '%s' not found in recipe source '%s' (%s)",
				recipeName,
				sourceName,
				checkout,
			)
		}
		return nil
	}
	return fmt.Errorf("recipe source '%s' is not declared in [[recipe_sources]]", sourceName)
}

func init() {
	rootCmd.AddCommand(enableCmd)
	rootCmd.AddCommand(disableCmd)
}
