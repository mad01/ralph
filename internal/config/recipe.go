package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gobwas/glob"
)

// RecipeFileName is the expected name of recipe files.
const RecipeFileName = "recipe.toml"

// LoadRecipe loads a recipe from the specified path.
func LoadRecipe(recipePath string) (*Recipe, error) {
	var recipe Recipe
	if _, err := toml.DecodeFile(recipePath, &recipe); err != nil {
		return nil, fmt.Errorf("failed to decode recipe file %s: %w", recipePath, err)
	}
	return &recipe, nil
}

// ResolveRecipePaths resolves relative paths in a recipe to be relative to the
// recipe's directory within the dotfiles repository.
// recipeDir is the directory containing the recipe file, relative to dotfiles_repo_path.
func ResolveRecipePaths(recipe *Recipe, recipeDir string) {
	// Resolve dotfile source paths
	for name, df := range recipe.Dotfiles {
		if df.Source != "" && !filepath.IsAbs(df.Source) {
			df.Source = filepath.Join(recipeDir, df.Source)
			recipe.Dotfiles[name] = df
		}
	}

	// Resolve tool config file source paths
	for i, tool := range recipe.Tools {
		for j, cf := range tool.ConfigFiles {
			if cf.Source != "" && !filepath.IsAbs(cf.Source) {
				recipe.Tools[i].ConfigFiles[j].Source = filepath.Join(recipeDir, cf.Source)
			}
		}
	}
}

// MergeRecipeIntoConfig merges a recipe's configuration items into the main config.
// Returns an error if there are naming conflicts (same key in multiple places).
// recipeName is used for error messages to identify which recipe caused conflicts.
func MergeRecipeIntoConfig(cfg *Config, recipe *Recipe, recipeName string) error {
	// Merge dotfiles
	if recipe.Dotfiles != nil {
		if cfg.Dotfiles == nil {
			cfg.Dotfiles = make(map[string]Dotfile)
		}
		for name, df := range recipe.Dotfiles {
			if _, exists := cfg.Dotfiles[name]; exists {
				return fmt.Errorf("dotfile '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Dotfiles[name] = df
		}
	}

	// Merge directories
	if recipe.Directories != nil {
		if cfg.Directories == nil {
			cfg.Directories = make(map[string]Directory)
		}
		for name, dir := range recipe.Directories {
			if _, exists := cfg.Directories[name]; exists {
				return fmt.Errorf("directory '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Directories[name] = dir
		}
	}

	// Merge repos
	if recipe.Repos != nil {
		if cfg.Repos == nil {
			cfg.Repos = make(map[string]Repo)
		}
		for name, repo := range recipe.Repos {
			if _, exists := cfg.Repos[name]; exists {
				return fmt.Errorf("repo '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Repos[name] = repo
		}
	}

	// Merge tools (append, no conflict detection for tools since they're a slice)
	cfg.Tools = append(cfg.Tools, recipe.Tools...)

	// Merge shell aliases
	if recipe.Shell.Aliases != nil {
		if cfg.Shell.Aliases == nil {
			cfg.Shell.Aliases = make(map[string]ShellAlias)
		}
		for name, alias := range recipe.Shell.Aliases {
			if _, exists := cfg.Shell.Aliases[name]; exists {
				return fmt.Errorf("shell alias '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Shell.Aliases[name] = alias
		}
	}

	// Merge shell functions
	if recipe.Shell.Functions != nil {
		if cfg.Shell.Functions == nil {
			cfg.Shell.Functions = make(map[string]ShellFunction)
		}
		for name, fn := range recipe.Shell.Functions {
			if _, exists := cfg.Shell.Functions[name]; exists {
				return fmt.Errorf("shell function '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Shell.Functions[name] = fn
		}
	}

	// Merge shell env vars
	if recipe.Shell.Env != nil {
		if cfg.Shell.Env == nil {
			cfg.Shell.Env = make(map[string]string)
		}
		for name, val := range recipe.Shell.Env {
			if _, exists := cfg.Shell.Env[name]; exists {
				return fmt.Errorf("shell env var '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Shell.Env[name] = val
		}
	}

	// Merge hooks - pre_apply and post_apply (append)
	cfg.Hooks.PreApply = append(cfg.Hooks.PreApply, recipe.Hooks.PreApply...)
	cfg.Hooks.PostApply = append(cfg.Hooks.PostApply, recipe.Hooks.PostApply...)

	// Merge pre_link hooks
	if recipe.Hooks.PreLink != nil {
		if cfg.Hooks.PreLink == nil {
			cfg.Hooks.PreLink = make(map[string][]string)
		}
		for name, hooks := range recipe.Hooks.PreLink {
			if _, exists := cfg.Hooks.PreLink[name]; exists {
				return fmt.Errorf("pre_link hook '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Hooks.PreLink[name] = hooks
		}
	}

	// Merge post_link hooks
	if recipe.Hooks.PostLink != nil {
		if cfg.Hooks.PostLink == nil {
			cfg.Hooks.PostLink = make(map[string][]string)
		}
		for name, hooks := range recipe.Hooks.PostLink {
			if _, exists := cfg.Hooks.PostLink[name]; exists {
				return fmt.Errorf("post_link hook '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Hooks.PostLink[name] = hooks
		}
	}

	// Merge builds
	if recipe.Hooks.Builds != nil {
		if cfg.Hooks.Builds == nil {
			cfg.Hooks.Builds = make(map[string]Build)
		}
		for name, build := range recipe.Hooks.Builds {
			if _, exists := cfg.Hooks.Builds[name]; exists {
				return fmt.Errorf("build '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.Hooks.Builds[name] = build
		}
	}

	// Merge template variables
	if recipe.TemplateVariables != nil {
		if cfg.TemplateVariables == nil {
			cfg.TemplateVariables = make(map[string]interface{})
		}
		for name, val := range recipe.TemplateVariables {
			if _, exists := cfg.TemplateVariables[name]; exists {
				return fmt.Errorf("template variable '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)", name, recipeName)
			}
			cfg.TemplateVariables[name] = val
		}
	}

	return nil
}

// DiscoverRecipes finds all recipe.toml files in the recipes directory.
// It applies exclusion patterns from RecipesConfig.Exclude.
func DiscoverRecipes(dotfilesRepoPath string, recipesConfig RecipesConfig) ([]RecipeRef, error) {
	expandedRepoPath, err := ExpandPath(dotfilesRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand dotfiles repo path: %w", err)
	}

	// Determine recipes directory
	recipesDir := recipesConfig.Dir
	if recipesDir == "" {
		recipesDir = DefaultRecipesDir
	}
	searchPath := filepath.Join(expandedRepoPath, recipesDir)

	// Check if recipes directory exists
	if _, err := os.Stat(searchPath); os.IsNotExist(err) {
		return nil, nil // No recipes directory, return empty list
	}

	// Compile exclusion patterns
	var excludeGlobs []glob.Glob
	for _, pattern := range recipesConfig.Exclude {
		g, err := glob.Compile(pattern, filepath.Separator)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern '%s': %w", pattern, err)
		}
		excludeGlobs = append(excludeGlobs, g)
	}

	var recipes []RecipeRef

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-files and non-recipe.toml files
		if info.IsDir() || info.Name() != RecipeFileName {
			return nil
		}

		// Get path relative to dotfiles repo (not to recipes dir)
		relPath, err := filepath.Rel(expandedRepoPath, path)
		if err != nil {
			return err
		}

		// Get path relative to recipes dir for exclusion matching
		relToRecipes, err := filepath.Rel(searchPath, path)
		if err != nil {
			return err
		}

		// Check against exclusion patterns
		for _, g := range excludeGlobs {
			if g.Match(relToRecipes) {
				return nil // Skip excluded paths
			}
		}

		// Create recipe ref with the short name (directory name within recipes/)
		dirName := filepath.Dir(relToRecipes)
		ref := RecipeRef{
			Path: relPath,
			Name: dirName, // Store the short name for override lookups
		}

		// Apply overrides if present (use directory name as key)
		if override, ok := recipesConfig.Overrides[dirName]; ok {
			ref.Enable = override.Enable
			ref.Hosts = override.Hosts
		}

		recipes = append(recipes, ref)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error discovering recipes: %w", err)
	}

	return recipes, nil
}

// ResolveRecipeRefPath resolves a RecipeRef to its full path.
// If Name is set, it looks for recipes/<name>/recipe.toml.
// If Path is set, it uses that directly.
func ResolveRecipeRefPath(ref RecipeRef, recipesDir string) string {
	if ref.Path != "" {
		return ref.Path
	}
	if ref.Name != "" {
		if recipesDir == "" {
			recipesDir = DefaultRecipesDir
		}
		return filepath.Join(recipesDir, ref.Name, RecipeFileName)
	}
	return ""
}

// ProcessRecipes loads and merges all enabled recipes into the config.
// It handles both explicit recipe lists and auto-discovery mode.
func ProcessRecipes(cfg *Config, currentHost string) error {
	expandedRepoPath, err := ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return fmt.Errorf("failed to expand dotfiles repo path: %w", err)
	}

	// Determine recipes directory
	recipesDir := cfg.RecipesConfig.Dir
	if recipesDir == "" {
		recipesDir = DefaultRecipesDir
	}

	var recipeRefs []RecipeRef

	// Determine which mode to use
	if cfg.RecipesConfig.AutoDiscover {
		// Auto-discovery mode
		discovered, err := DiscoverRecipes(cfg.DotfilesRepoPath, cfg.RecipesConfig)
		if err != nil {
			return fmt.Errorf("recipe auto-discovery failed: %w", err)
		}
		recipeRefs = discovered
	} else if len(cfg.Recipes) > 0 {
		// Explicit mode - resolve short names to paths
		for _, ref := range cfg.Recipes {
			resolvedRef := ref
			resolvedRef.Path = ResolveRecipeRefPath(ref, recipesDir)
			recipeRefs = append(recipeRefs, resolvedRef)
		}
	} else {
		// No recipes configured
		return nil
	}

	// Process each recipe
	for _, ref := range recipeRefs {
		// Check if recipe is enabled
		if !IsEnabled(ref.Enable) {
			continue
		}

		// Check host filter for recipe
		if !ShouldApplyForHost(ref.Hosts, currentHost) {
			continue
		}

		// Load the recipe
		recipePath := filepath.Join(expandedRepoPath, ref.Path)
		recipe, err := LoadRecipe(recipePath)
		if err != nil {
			return fmt.Errorf("failed to load recipe '%s': %w", ref.Path, err)
		}

		// Get the directory containing the recipe
		recipeDir := filepath.Dir(ref.Path)

		// Resolve relative paths
		ResolveRecipePaths(recipe, recipeDir)

		// Apply recipe-level host filter to items that don't have their own
		applyRecipeHostFilter(recipe, ref.Hosts)

		// Get recipe name for error messages
		recipeName := recipe.Recipe.Name
		if recipeName == "" {
			if ref.Name != "" {
				recipeName = ref.Name
			} else {
				recipeName = ref.Path
			}
		}

		// Merge into config
		if err := MergeRecipeIntoConfig(cfg, recipe, recipeName); err != nil {
			return err
		}

		// Store loaded recipe info for migration support
		cfg.LoadedRecipes = append(cfg.LoadedRecipes, LoadedRecipeInfo{
			Path:        ref.Path,
			Dir:         recipeDir,
			Name:        recipeName,
			LegacyPaths: recipe.Recipe.LegacyPaths,
		})
	}

	return nil
}

// applyRecipeHostFilter applies the recipe-level host filter to items that
// don't have their own host filter specified.
func applyRecipeHostFilter(recipe *Recipe, recipeHosts []string) {
	if len(recipeHosts) == 0 {
		return // No recipe-level filter to apply
	}

	// Apply to dotfiles
	for name, df := range recipe.Dotfiles {
		if len(df.Hosts) == 0 {
			df.Hosts = recipeHosts
			recipe.Dotfiles[name] = df
		}
	}

	// Apply to directories
	for name, dir := range recipe.Directories {
		if len(dir.Hosts) == 0 {
			dir.Hosts = recipeHosts
			recipe.Directories[name] = dir
		}
	}

	// Apply to repos
	for name, repo := range recipe.Repos {
		if len(repo.Hosts) == 0 {
			repo.Hosts = recipeHosts
			recipe.Repos[name] = repo
		}
	}

	// Apply to tools
	for i := range recipe.Tools {
		if len(recipe.Tools[i].Hosts) == 0 {
			recipe.Tools[i].Hosts = recipeHosts
		}
	}

	// Apply to shell aliases
	for name, alias := range recipe.Shell.Aliases {
		if len(alias.Hosts) == 0 {
			alias.Hosts = recipeHosts
			recipe.Shell.Aliases[name] = alias
		}
	}

	// Apply to shell functions
	for name, fn := range recipe.Shell.Functions {
		if len(fn.Hosts) == 0 {
			fn.Hosts = recipeHosts
			recipe.Shell.Functions[name] = fn
		}
	}

	// Apply to builds
	for name, build := range recipe.Hooks.Builds {
		if len(build.Hosts) == 0 {
			build.Hosts = recipeHosts
			recipe.Hooks.Builds[name] = build
		}
	}
}

// GetAllLegacyPaths returns a consolidated map of all legacy paths from all
// loaded recipes. The map keys are old source paths (relative to dotfiles repo)
// and values are new source paths.
func GetAllLegacyPaths(cfg *Config) map[string]string {
	result := make(map[string]string)
	for _, info := range cfg.LoadedRecipes {
		for oldPath, newPath := range info.LegacyPaths {
			// Resolve the new path relative to the recipe directory
			if !filepath.IsAbs(newPath) && !strings.HasPrefix(newPath, info.Dir) {
				newPath = filepath.Join(info.Dir, newPath)
			}
			result[oldPath] = newPath
		}
	}
	return result
}
