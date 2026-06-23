package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// LocalConfigSuffix is inserted before the extension of the main config file to
// derive the optional, git-ignored overlay path: config.toml -> config.local.toml.
const LocalConfigSuffix = ".local"

// LocalConfigFileName is the overlay file name derived from the default
// config.toml. Used by `ralph init` for the .gitignore entry.
const LocalConfigFileName = "config" + LocalConfigSuffix + ".toml"

// LocalConfigPath derives the overlay path from the resolved main config path.
// It keeps the directory and base name and inserts ".local" before the
// extension, mirroring mise's config.toml -> config.local.toml convention.
// A main path with no extension gets ".local.toml" appended.
func LocalConfigPath(mainConfigPath string) string {
	dir := filepath.Dir(mainConfigPath)
	base := filepath.Base(mainConfigPath)
	ext := filepath.Ext(base)
	if ext == "" {
		return filepath.Join(dir, base+LocalConfigSuffix+".toml")
	}
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+LocalConfigSuffix+ext)
}

// loadLocalOverlay decodes the optional config.local.toml sitting next to the
// main config and merges it onto cfg. The overlay is always optional: a missing
// file is not an error. It returns whether an overlay was found and merged.
//
// Merge semantics (local wins, never conflict-detected like recipes):
//   - scalar fields: local overrides when set to a non-zero value
//   - map fields: keys are merged, local wins per key
//   - slice fields: local replaces the whole slice when non-empty
func loadLocalOverlay(cfg *Config, mainConfigPath string) (bool, error) {
	localPath := LocalConfigPath(mainConfigPath)

	if _, err := os.Stat(localPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat local config %s: %w", localPath, err)
	}

	var local Config
	if _, err := toml.DecodeFile(localPath, &local); err != nil {
		return false, fmt.Errorf("failed to decode local config %s: %w", localPath, err)
	}

	mergeLocalConfig(cfg, &local)
	return true, nil
}

// mergeLocalConfig overlays local onto base in place with local-wins semantics.
func mergeLocalConfig(base, local *Config) {
	// Scalars: local overrides when set.
	if local.DotfilesRepoPath != "" {
		base.DotfilesRepoPath = local.DotfilesRepoPath
	}
	if local.PackagesDir != "" {
		base.PackagesDir = local.PackagesDir
	}
	if local.Shell.Name != "" {
		base.Shell.Name = local.Shell.Name
	}
	if local.RecipesConfig.AutoDiscover {
		base.RecipesConfig.AutoDiscover = true
	}
	if local.RecipesConfig.AutoCleanup {
		base.RecipesConfig.AutoCleanup = true
	}
	if local.RecipesConfig.Dir != "" {
		base.RecipesConfig.Dir = local.RecipesConfig.Dir
	}

	// Maps: merge keys, local wins per key.
	mergeLocalMap(&base.Dotfiles, local.Dotfiles)
	mergeLocalMap(&base.DirsMirror, local.DirsMirror)
	mergeLocalMap(&base.Directories, local.Directories)
	mergeLocalMap(&base.Repos, local.Repos)
	mergeLocalMap(&base.Packages, local.Packages)
	mergeLocalMap(&base.TemplateVariables, local.TemplateVariables)
	mergeLocalMap(&base.Shell.Aliases, local.Shell.Aliases)
	mergeLocalMap(&base.Shell.Functions, local.Shell.Functions)
	mergeLocalMap(&base.Shell.Env, local.Shell.Env)
	mergeLocalMap(&base.Hooks.PreLink, local.Hooks.PreLink)
	mergeLocalMap(&base.Hooks.PostLink, local.Hooks.PostLink)
	mergeLocalMap(&base.Hooks.Builds, local.Hooks.Builds)
	mergeLocalMap(&base.RecipesConfig.Overrides, local.RecipesConfig.Overrides)

	// Slices: local replaces the whole slice when set.
	if len(local.Tools) > 0 {
		base.Tools = local.Tools
	}
	if len(local.Recipes) > 0 {
		base.Recipes = local.Recipes
	}
	if len(local.Hooks.PreApply) > 0 {
		base.Hooks.PreApply = local.Hooks.PreApply
	}
	if len(local.Hooks.PostApply) > 0 {
		base.Hooks.PostApply = local.Hooks.PostApply
	}
	if len(local.Hooks.PreUninstall) > 0 {
		base.Hooks.PreUninstall = local.Hooks.PreUninstall
	}
	if len(local.Hooks.PostUninstall) > 0 {
		base.Hooks.PostUninstall = local.Hooks.PostUninstall
	}
	if len(local.RecipesConfig.Exclude) > 0 {
		base.RecipesConfig.Exclude = local.RecipesConfig.Exclude
	}
	if len(local.Profiles) > 0 {
		base.Profiles = local.Profiles
	}
}

// mergeLocalMap copies every key from local into *base, allocating the base map
// if needed. Local values overwrite existing keys.
func mergeLocalMap[T any](base *map[string]T, local map[string]T) {
	if len(local) == 0 {
		return
	}
	if *base == nil {
		*base = make(map[string]T, len(local))
	}
	maps.Copy(*base, local)
}
