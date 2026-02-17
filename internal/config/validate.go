package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateConfig performs basic validation on the loaded configuration.
func ValidateConfig(cfg *Config) error {
	if cfg.DotfilesRepoPath == "" {
		return fmt.Errorf("dotfiles_repo_path cannot be empty")
	}

	// Basic validation for DotfilesRepoPath (e.g., check if it's an absolute path after expansion)
	// More sophisticated checks (like directory existence) might be done by the consuming logic (e.g., apply command)
	expandedRepoPath, err := ExpandPath(cfg.DotfilesRepoPath)
	if err != nil {
		return fmt.Errorf("error expanding dotfiles_repo_path '%s': %w", cfg.DotfilesRepoPath, err)
	}
	if !filepath.IsAbs(expandedRepoPath) {
		// This check might be too strict, as user might provide a relative path
		// intending it to be relative to some base, but for a repo path, absolute is safer.
		// Consider if this should be a warning or handled differently.
		// For now, let's assume it should resolve to an absolute path.
		// return fmt.Errorf("dotfiles_repo_path '%s' (expanded: '%s') must be an absolute path", cfg.DotfilesRepoPath, expandedRepoPath)
	}

	for name, df := range cfg.Dotfiles {
		if df.Source == "" {
			return fmt.Errorf("dotfile item '%s': source cannot be empty", name)
		}
		if df.Target == "" {
			return fmt.Errorf("dotfile item '%s': target cannot be empty", name)
		}
		// Validate action field
		if df.Action != "" && df.Action != "symlink" && df.Action != "copy" && df.Action != "symlink_dir" {
			return fmt.Errorf("dotfile item '%s': action must be 'symlink', 'copy', or 'symlink_dir', got '%s'", name, df.Action)
		}
		// Target should ideally be an absolute path after expansion
		expandedTarget, err := ExpandPath(df.Target)
		if err != nil {
			return fmt.Errorf("dotfile item '%s': error expanding target path '%s': %w", name, df.Target, err)
		}
		if !filepath.IsAbs(expandedTarget) {
			// return fmt.Errorf("dotfile item '%s': target path '%s' (expanded: '%s') must be an absolute path", name, df.Target, expandedTarget)
		}
	}

	// Validate directories
	for name, dir := range cfg.Directories {
		if dir.Target == "" {
			return fmt.Errorf("directory '%s': target cannot be empty", name)
		}
		_, err := ExpandPath(dir.Target)
		if err != nil {
			return fmt.Errorf("directory '%s': error expanding target path '%s': %w", name, dir.Target, err)
		}
	}

	// Validate repos
	for name, repo := range cfg.Repos {
		if repo.URL == "" {
			return fmt.Errorf("repo '%s': url cannot be empty", name)
		}
		if repo.Target == "" {
			return fmt.Errorf("repo '%s': target cannot be empty", name)
		}
		// update and commit are mutually exclusive
		if repo.Update && repo.Commit != "" {
			return fmt.Errorf("repo '%s': update and commit are mutually exclusive (can't pull latest AND pin to commit)", name)
		}
		_, err := ExpandPath(repo.Target)
		if err != nil {
			return fmt.Errorf("repo '%s': error expanding target path '%s': %w", name, repo.Target, err)
		}
	}

	for i, tool := range cfg.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tool at index %d: name cannot be empty", i)
		}
		if tool.CheckCommand == "" {
			return fmt.Errorf("tool '%s': check_command cannot be empty", tool.Name)
		}
		for j, cf := range tool.ConfigFiles {
			if cf.Source == "" {
				return fmt.Errorf("tool '%s', config file at index %d: source cannot be empty", tool.Name, j)
			}
			if cf.Target == "" {
				return fmt.Errorf("tool '%s', config file at index %d: target cannot be empty", tool.Name, j)
			}
		}
	}

	for aliasName, alias := range cfg.Shell.Aliases {
		if alias.Command == "" {
			return fmt.Errorf("shell alias '%s': command cannot be empty", aliasName)
		}
	}

	for funcName, shellFunc := range cfg.Shell.Functions {
		if shellFunc.Body == "" {
			return fmt.Errorf("shell function '%s': body cannot be empty", funcName)
		}
	}

	// Validate build hooks
	for name, build := range cfg.Hooks.Builds {
		if len(build.Commands) == 0 {
			return fmt.Errorf("build '%s': commands cannot be empty", name)
		}
		if build.Run == "" {
			return fmt.Errorf("build '%s': run mode is required (always, once, or manual)", name)
		}
		if build.Run != "always" && build.Run != "once" && build.Run != "manual" {
			return fmt.Errorf("build '%s': run mode must be 'always', 'once', or 'manual', got '%s'", name, build.Run)
		}
	}

	// Validate recipe references
	for i, ref := range cfg.Recipes {
		if ref.Path == "" && ref.Name == "" {
			return fmt.Errorf("recipe at index %d: either 'name' or 'path' must be specified", i)
		}
	}

	return nil
}

// ValidateMergedConfig performs validation on the merged configuration
// (after recipes have been processed). This validates the consistency
// of the complete configuration.
func ValidateMergedConfig(cfg *Config) error {
	// Validate all dotfiles (including those from recipes)
	for name, df := range cfg.Dotfiles {
		if df.Source == "" {
			return fmt.Errorf("dotfile item '%s': source cannot be empty", name)
		}
		if df.Target == "" {
			return fmt.Errorf("dotfile item '%s': target cannot be empty", name)
		}
		if df.Action != "" && df.Action != "symlink" && df.Action != "copy" && df.Action != "symlink_dir" {
			return fmt.Errorf("dotfile item '%s': action must be 'symlink', 'copy', or 'symlink_dir', got '%s'", name, df.Action)
		}
		expandedTarget, err := ExpandPath(df.Target)
		if err != nil {
			return fmt.Errorf("dotfile item '%s': error expanding target path '%s': %w", name, df.Target, err)
		}
		_ = expandedTarget // Used for validation
	}

	// Validate all directories
	for name, dir := range cfg.Directories {
		if dir.Target == "" {
			return fmt.Errorf("directory '%s': target cannot be empty", name)
		}
		_, err := ExpandPath(dir.Target)
		if err != nil {
			return fmt.Errorf("directory '%s': error expanding target path '%s': %w", name, dir.Target, err)
		}
	}

	// Validate all repos
	for name, repo := range cfg.Repos {
		if repo.URL == "" {
			return fmt.Errorf("repo '%s': url cannot be empty", name)
		}
		if repo.Target == "" {
			return fmt.Errorf("repo '%s': target cannot be empty", name)
		}
		if repo.Update && repo.Commit != "" {
			return fmt.Errorf("repo '%s': update and commit are mutually exclusive", name)
		}
		_, err := ExpandPath(repo.Target)
		if err != nil {
			return fmt.Errorf("repo '%s': error expanding target path '%s': %w", name, repo.Target, err)
		}
	}

	// Validate all tools
	for i, tool := range cfg.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tool at index %d: name cannot be empty", i)
		}
		if tool.CheckCommand == "" {
			return fmt.Errorf("tool '%s': check_command cannot be empty", tool.Name)
		}
		for j, cf := range tool.ConfigFiles {
			if cf.Source == "" {
				return fmt.Errorf("tool '%s', config file at index %d: source cannot be empty", tool.Name, j)
			}
			if cf.Target == "" {
				return fmt.Errorf("tool '%s', config file at index %d: target cannot be empty", tool.Name, j)
			}
		}
	}

	// Validate all shell aliases
	for aliasName, alias := range cfg.Shell.Aliases {
		if alias.Command == "" {
			return fmt.Errorf("shell alias '%s': command cannot be empty", aliasName)
		}
	}

	// Validate all shell functions
	for funcName, shellFunc := range cfg.Shell.Functions {
		if shellFunc.Body == "" {
			return fmt.Errorf("shell function '%s': body cannot be empty", funcName)
		}
	}

	// Validate all builds
	for name, build := range cfg.Hooks.Builds {
		if len(build.Commands) == 0 {
			return fmt.Errorf("build '%s': commands cannot be empty", name)
		}
		if build.Run == "" {
			return fmt.Errorf("build '%s': run mode is required (always, once, or manual)", name)
		}
		if build.Run != "always" && build.Run != "once" && build.Run != "manual" {
			return fmt.Errorf("build '%s': run mode must be 'always', 'once', or 'manual', got '%s'", name, build.Run)
		}
	}

	return nil
}

// ShortenHome replaces the user's home directory prefix with ~ for display.
func ShortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// ExpandPath expands ~ and environment variables in a path.
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[1:])
	}
	return os.ExpandEnv(path), nil
}
