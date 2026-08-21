package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gobwas/glob"
)

// RecipeFileName is the expected name of recipe files.
const RecipeFileName = "recipe.toml"

// LoadRecipe loads a recipe from the specified path. Unknown keys are legal
// TOML but dead configuration — a mistyped table parses cleanly and applies
// nothing (a [symlinks] table once cost three overlay recipes all their
// items) — so they warn on stderr instead of passing silently.
func LoadRecipe(recipePath string) (*Recipe, error) {
	var recipe Recipe
	md, err := toml.DecodeFile(recipePath, &recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to decode recipe file %s: %w", recipePath, err)
	}
	if summary := undecodedSummary(md.Undecoded()); summary != "" {
		fmt.Fprintf(
			os.Stderr,
			"Warning: recipe %s: unknown keys ignored (not in the recipe schema): %s\n",
			recipePath,
			summary,
		)
	}
	return &recipe, nil
}

// undecodedSummary compresses undecoded TOML keys into a short, deduplicated,
// sorted list. Keys are cut to their first two segments so one mistyped table
// with many fields reports once ("symlinks.myitem" instead of every field),
// and lists longer than five entries are capped.
func undecodedSummary(keys []toml.Key) string {
	seen := map[string]bool{}
	var names []string
	for _, k := range keys {
		name := k.String()
		if len(k) > 2 {
			name = strings.Join(k[:2], ".")
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) > 5 {
		names = append(names[:5], fmt.Sprintf("+%d more", len(names)-5))
	}
	return strings.Join(names, ", ")
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

	// Resolve dirs_mirror source paths
	for name, dm := range recipe.DirsMirror {
		if dm.Source != "" && !filepath.IsAbs(dm.Source) {
			dm.Source = filepath.Join(recipeDir, dm.Source)
			recipe.DirsMirror[name] = dm
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
// mergeRecipeMap merges src into *dst, failing on any key already present
// (conflicts across recipes/main config are not allowed). kind labels the item
// type in conflict errors. stamp, if non-nil, is applied to each value before
// insertion (e.g. to set OwnerRecipe/Wave).
func mergeRecipeMap[T any](
	dst *map[string]T,
	src map[string]T,
	kind, recipeName string,
	stamp func(*T),
) error {
	if src == nil {
		return nil
	}
	if *dst == nil {
		*dst = make(map[string]T, len(src))
	}
	for name, v := range src {
		if _, exists := (*dst)[name]; exists {
			return fmt.Errorf(
				"%s '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)",
				kind,
				name,
				recipeName,
			)
		}
		if stamp != nil {
			stamp(&v)
		}
		(*dst)[name] = v
	}
	return nil
}

func MergeRecipeIntoConfig(cfg *Config, recipe *Recipe, recipeName string) error {
	effectiveWave := 1
	if recipe.Recipe.Wave != nil {
		effectiveWave = *recipe.Recipe.Wave
	}

	merges := []func() error{
		func() error {
			return mergeRecipeMap(
				&cfg.Dotfiles,
				recipe.Dotfiles,
				"dotfile",
				recipeName,
				func(d *Dotfile) { d.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.DirsMirror,
				recipe.DirsMirror,
				"dirs_mirror",
				recipeName,
				func(d *DirMirror) { d.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Directories,
				recipe.Directories,
				"directory",
				recipeName,
				func(d *Directory) { d.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Repos,
				recipe.Repos,
				"repo",
				recipeName,
				func(r *Repo) { r.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Shell.Aliases,
				recipe.Shell.Aliases,
				"shell alias",
				recipeName,
				func(a *ShellAlias) { a.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Shell.Functions,
				recipe.Shell.Functions,
				"shell function",
				recipeName,
				func(f *ShellFunction) { f.OwnerRecipe = recipeName },
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Hooks.PreLink,
				recipe.Hooks.PreLink,
				"pre_link hook",
				recipeName,
				nil,
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Hooks.PostLink,
				recipe.Hooks.PostLink,
				"post_link hook",
				recipeName,
				nil,
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Hooks.Builds,
				recipe.Hooks.Builds,
				"build",
				recipeName,
				func(b *Build) {
					b.OwnerRecipe = recipeName
					b.Wave = effectiveWave
				},
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.Packages,
				recipe.Packages,
				"package",
				recipeName,
				func(p *Package) {
					p.OwnerRecipe = recipeName
					p.Wave = effectiveWave
				},
			)
		},
		func() error {
			return mergeRecipeMap(
				&cfg.TemplateVariables,
				recipe.TemplateVariables,
				"template variable",
				recipeName,
				nil,
			)
		},
	}
	for _, merge := range merges {
		if err := merge(); err != nil {
			return err
		}
	}

	// Tools are a slice (append, no conflict detection).
	for i := range recipe.Tools {
		recipe.Tools[i].OwnerRecipe = recipeName
	}
	cfg.Tools = append(cfg.Tools, recipe.Tools...)

	// Shell env vars track ownership in a parallel EnvOwners map.
	if recipe.Shell.Env != nil {
		if cfg.Shell.Env == nil {
			cfg.Shell.Env = make(map[string]string)
		}
		if cfg.Shell.EnvOwners == nil {
			cfg.Shell.EnvOwners = make(map[string]string)
		}
		for name, val := range recipe.Shell.Env {
			if _, exists := cfg.Shell.Env[name]; exists {
				return fmt.Errorf(
					"shell env var '%s' defined in multiple locations: recipe '%s' and main config (or another recipe)",
					name,
					recipeName,
				)
			}
			cfg.Shell.Env[name] = val
			cfg.Shell.EnvOwners[name] = recipeName
		}
	}

	// pre_apply / post_apply hooks are ordered slices (append).
	cfg.Hooks.PreApply = append(cfg.Hooks.PreApply, recipe.Hooks.PreApply...)
	cfg.Hooks.PostApply = append(cfg.Hooks.PostApply, recipe.Hooks.PostApply...)

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

// resolveRecipeName returns the canonical name for a recipe: the name from
// its metadata when available, otherwise the ref's short name, otherwise its
// path. recipe may be nil (e.g. when an off-host recipe failed to load).
func resolveRecipeName(recipe *Recipe, ref RecipeRef) string {
	if recipe != nil && recipe.Recipe.Name != "" {
		return recipe.Recipe.Name
	}
	if ref.Name != "" {
		return ref.Name
	}
	return ref.Path
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

			// Honor [recipes_config.overrides.<dir>] for explicit refs too, so
			// `ralph enable`/`disable` (which write these overrides) work
			// regardless of whether a recipe is auto-discovered or explicitly
			// referenced. The override key is the recipe's directory name,
			// matching auto-discovery and the enable/disable commands.
			dirName := filepath.Base(filepath.Dir(resolvedRef.Path))
			if override, ok := cfg.RecipesConfig.Overrides[dirName]; ok {
				if override.Enable != nil {
					resolvedRef.Enable = override.Enable
				}
				if len(override.Hosts) > 0 {
					resolvedRef.Hosts = override.Hosts
				}
			}

			recipeRefs = append(recipeRefs, resolvedRef)
		}
	}
	// No local recipes configured is fine — remote recipe sources below may
	// still contribute recipes.

	// Process each recipe
	for _, ref := range recipeRefs {
		// Vars overrides key on the recipe directory name, like enable/hosts.
		dirName := filepath.Base(filepath.Dir(ref.Path))
		// Local recipes resolve paths relative to the dotfiles repo.
		if err := processRecipeRef(
			cfg,
			ref,
			expandedRepoPath,
			filepath.Dir(ref.Path),
			"",
			currentHost,
			cfg.RecipesConfig.Overrides[dirName].Vars,
		); err != nil {
			return err
		}
	}

	return processRecipeSources(cfg, currentHost)
}

// processRecipeRef loads one recipe ref rooted at loadRoot and merges it into
// cfg. resolveDir is prefixed onto recipe-relative paths: the recipe dir
// relative to the dotfiles repo for local recipes, or the absolute recipe dir
// inside a source checkout (making merged paths absolute, which the apply
// layer passes through via JoinSourcePath). namePrefix namespaces the recipe
// identity ("<source>/") for remote sources.
func processRecipeRef(
	cfg *Config,
	ref RecipeRef,
	loadRoot, resolveDir, namePrefix string,
	currentHost string,
	overrideVars map[string]string,
) error {
	// Disabled recipes are intentionally cleaned up — skip entirely.
	if !IsEnabled(ref.Enable) {
		return nil
	}

	// Load the recipe.
	recipePath := filepath.Join(loadRoot, ref.Path)
	recipe, err := LoadRecipe(recipePath)

	// Host-filtered recipes belong to other hosts: don't apply them here,
	// but record their name so cleanup freezes (rather than deletes) any
	// artifacts a previous apply on a matching host recorded. A malformed
	// off-host recipe must not break apply, so fall back to a ref-derived
	// name if it failed to load.
	if !ShouldApplyForHost(ref.Hosts, currentHost) {
		cfg.HostFilteredRecipes = append(
			cfg.HostFilteredRecipes,
			namePrefix+resolveRecipeName(recipe, ref),
		)
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to load recipe '%s': %w", ref.Path, err)
	}

	// Profile-filtered recipes belong to other machine profiles: freeze rather
	// than apply, same as host-filtered recipes. Machine profiles come from the
	// config.local.toml overlay (cfg.Profiles). Unlike the host filter, this
	// must run after the load-error check above: a recipe's profiles live in
	// its recipe.toml, so the recipe has to parse before we can read them — an
	// unparseable on-host recipe is a hard error regardless of profile.
	if !ShouldApplyForProfiles(recipe.Recipe.Profiles, cfg.Profiles) {
		cfg.HostFilteredRecipes = append(
			cfg.HostFilteredRecipes,
			namePrefix+resolveRecipeName(recipe, ref),
		)
		return nil
	}

	// Resolve relative paths
	ResolveRecipePaths(recipe, resolveDir)

	// Apply recipe-level host filter to items that don't have their own
	applyRecipeHostFilter(recipe, ref.Hosts)

	// Get recipe name for error messages
	recipeName := namePrefix + resolveRecipeName(recipe, ref)

	// Expand {{vars.<name>}} placeholders before merging so the final
	// names take part in duplicate detection and name validation.
	if err := ExpandRecipeVars(recipe, overrideVars); err != nil {
		return fmt.Errorf("recipe '%s': %w", recipeName, err)
	}

	// Merge into config
	if err := MergeRecipeIntoConfig(cfg, recipe, recipeName); err != nil {
		return err
	}

	// Store loaded recipe info for migration + cleanup support
	deleteBehavior := recipe.Recipe.DeleteBehavior
	if deleteBehavior == "" {
		deleteBehavior = DeleteBehaviorDelete
	}
	loadedWave := 1
	if recipe.Recipe.Wave != nil {
		loadedWave = *recipe.Recipe.Wave
	}
	cfg.LoadedRecipes = append(cfg.LoadedRecipes, LoadedRecipeInfo{
		Path:           ref.Path,
		Dir:            resolveDir,
		Name:           recipeName,
		RecipeSource:   strings.TrimSuffix(namePrefix, "/"),
		LegacyPaths:    recipe.Recipe.LegacyPaths,
		DeleteBehavior: deleteBehavior,
		Wave:           loadedWave,
		Caveats:        recipe.Recipe.Caveats,
		PreUninstall:   recipe.Hooks.PreUninstall,
		PostUninstall:  recipe.Hooks.PostUninstall,
	})
	return nil
}

// processRecipeSources ensures each enabled remote recipe source has a cached
// checkout, discovers its recipes, and merges them with identity
// "<source>/<recipe>". Enable/hosts overrides apply via the namespaced key
// (e.g. [recipes_config.overrides."thismoon/reminder"]).
func processRecipeSources(cfg *Config, currentHost string) error {
	if len(cfg.RecipeSources) == 0 {
		return nil
	}

	sourcesDir, err := SourcesDir()
	if err != nil {
		return fmt.Errorf("failed to expand sources dir: %w", err)
	}

	for _, src := range cfg.RecipeSources {
		if !IsEnabled(src.Enable) {
			continue
		}
		if !ShouldApplyForProfiles(src.Profiles, cfg.Profiles) {
			// No checkout or discovery occurs for a source belonging to another
			// machine profile. Record the source separately so cleanup can use
			// persisted provenance instead of guessing from recipe names.
			cfg.ProfileFilteredRecipeSources = append(
				cfg.ProfileFilteredRecipeSources,
				src.Name,
			)
			continue
		}

		// Clone progress goes to stderr: config loading has no writer, and a
		// first-run bootstrap clone should not be silent.
		checkout, err := EnsureSourceCheckout(os.Stderr, src, sourcesDir)
		if err != nil {
			return err
		}

		refs, err := DiscoverRecipes(checkout, RecipesConfig{Dir: SourceRecipesDir(src)})
		if err != nil {
			return fmt.Errorf("recipe_source '%s': discovery failed: %w", src.Name, err)
		}

		for _, ref := range refs {
			if override, ok := cfg.RecipesConfig.Overrides[src.Name+"/"+ref.Name]; ok {
				ref.Enable = override.Enable
				ref.Hosts = override.Hosts
			}
			// Absolute resolveDir roots the recipe's paths in the checkout.
			if err := processRecipeRef(
				cfg,
				ref,
				checkout,
				filepath.Join(checkout, filepath.Dir(ref.Path)),
				src.Name+"/",
				currentHost,
				cfg.RecipesConfig.Overrides[src.Name+"/"+ref.Name].Vars,
			); err != nil {
				return err
			}
		}
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

	// Apply to dirs_mirror
	for name, dm := range recipe.DirsMirror {
		if len(dm.Hosts) == 0 {
			dm.Hosts = recipeHosts
			recipe.DirsMirror[name] = dm
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

	// Apply to tools and their config files
	for i := range recipe.Tools {
		if len(recipe.Tools[i].Hosts) == 0 {
			recipe.Tools[i].Hosts = recipeHosts
		}
		for j := range recipe.Tools[i].ConfigFiles {
			if len(recipe.Tools[i].ConfigFiles[j].Hosts) == 0 {
				recipe.Tools[i].ConfigFiles[j].Hosts = recipeHosts
			}
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

	// Apply to packages
	for name, pkg := range recipe.Packages {
		if len(pkg.Hosts) == 0 {
			pkg.Hosts = recipeHosts
			recipe.Packages[name] = pkg
		}
	}

	// Note: recipe.Shell.Env is a flat map[string]string with no per-entry
	// host field, so it cannot inherit a host filter here. Recipe-level host
	// filtering (in ProcessRecipes) already prevents an off-host recipe's env
	// from being merged at all; finer per-var host scoping would require
	// upgrading Env to a typed struct.
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
