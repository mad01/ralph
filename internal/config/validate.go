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
		if df.Action != "" && df.Action != "symlink" && df.Action != "copy" {
			return fmt.Errorf("dotfile item '%s': action must be 'symlink' or 'copy', got '%s'", name, df.Action)
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

	for aliasName, aliasCmd := range cfg.Shell.Aliases {
		if aliasCmd == "" {
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

	return nil
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
