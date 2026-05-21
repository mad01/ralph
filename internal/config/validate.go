package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateDotfiles validates a map of Dotfile entries.
func validateDotfiles(dotfiles map[string]Dotfile) error {
	for name, df := range dotfiles {
		if df.Source == "" {
			return fmt.Errorf("dotfile item '%s': source cannot be empty", name)
		}
		if df.Target == "" {
			return fmt.Errorf("dotfile item '%s': target cannot be empty", name)
		}
		if df.Action != "" && df.Action != "symlink" && df.Action != "copy" && df.Action != "symlink_dir" {
			return fmt.Errorf("dotfile item '%s': action must be 'symlink', 'copy', or 'symlink_dir', got '%s'", name, df.Action)
		}
		_, err := ExpandPath(df.Target)
		if err != nil {
			return fmt.Errorf("dotfile item '%s': error expanding target path '%s': %w", name, df.Target, err)
		}
	}
	return nil
}

// validateDirectories validates a map of Directory entries.
func validateDirectories(dirs map[string]Directory) error {
	for name, dir := range dirs {
		if dir.Target == "" {
			return fmt.Errorf("directory '%s': target cannot be empty", name)
		}
		_, err := ExpandPath(dir.Target)
		if err != nil {
			return fmt.Errorf("directory '%s': error expanding target path '%s': %w", name, dir.Target, err)
		}
	}
	return nil
}

// validateRepos validates a map of Repo entries.
func validateRepos(repos map[string]Repo) error {
	for name, repo := range repos {
		if repo.URL == "" {
			return fmt.Errorf("repo '%s': url cannot be empty", name)
		}
		if repo.Target == "" {
			return fmt.Errorf("repo '%s': target cannot be empty", name)
		}
		if repo.Update && repo.Commit != "" {
			return fmt.Errorf("repo '%s': update and commit are mutually exclusive (can't pull latest AND pin to commit)", name)
		}
		_, err := ExpandPath(repo.Target)
		if err != nil {
			return fmt.Errorf("repo '%s': error expanding target path '%s': %w", name, repo.Target, err)
		}
	}
	return nil
}

// validateTools validates a slice of Tool entries.
func validateTools(tools []Tool) error {
	for i, tool := range tools {
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
	return nil
}

// validateAliases validates a map of ShellAlias entries.
func validateAliases(aliases map[string]ShellAlias) error {
	for aliasName, alias := range aliases {
		if alias.Command == "" {
			return fmt.Errorf("shell alias '%s': command cannot be empty", aliasName)
		}
	}
	return nil
}

// validateFunctions validates a map of ShellFunction entries.
func validateFunctions(funcs map[string]ShellFunction) error {
	for funcName, shellFunc := range funcs {
		if shellFunc.Body == "" {
			return fmt.Errorf("shell function '%s': body cannot be empty", funcName)
		}
	}
	return nil
}

// validateBuilds validates a map of Build entries.
func validateBuilds(builds map[string]Build) error {
	for name, build := range builds {
		if build.Script != "" && len(build.Commands) > 0 {
			return fmt.Errorf("build '%s': script and commands are mutually exclusive", name)
		}
		if build.Script == "" && len(build.Commands) == 0 {
			return fmt.Errorf("build '%s': either script or commands must be provided", name)
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

// validatePackages validates a map of Package entries.
func validatePackages(pkgs map[string]Package) error {
	for name, pkg := range pkgs {
		if pkg.Source == "" {
			return fmt.Errorf("package '%s': source is required (local or remote)", name)
		}
		if pkg.Source != "local" && pkg.Source != "remote" {
			return fmt.Errorf("package '%s': source must be 'local' or 'remote', got '%s'", name, pkg.Source)
		}
		if pkg.Source == "remote" && pkg.Repo == "" {
			return fmt.Errorf("package '%s': repo is required for remote packages", name)
		}
		if pkg.Source == "local" && pkg.WorkingDir == "" {
			return fmt.Errorf("package '%s': working_dir is required for local packages", name)
		}
		if len(pkg.Build) == 0 {
			return fmt.Errorf("package '%s': at least one build command is required", name)
		}
	}
	return nil
}

// ValidateConfig performs basic validation on the loaded configuration.
func ValidateConfig(cfg *Config) error {
	if cfg.DotfilesRepoPath == "" {
		return fmt.Errorf("dotfiles_repo_path cannot be empty")
	}

	if _, err := ExpandPath(cfg.DotfilesRepoPath); err != nil {
		return fmt.Errorf("error expanding dotfiles_repo_path '%s': %w", cfg.DotfilesRepoPath, err)
	}

	if err := validateDotfiles(cfg.Dotfiles); err != nil {
		return err
	}
	if err := validateDirectories(cfg.Directories); err != nil {
		return err
	}
	if err := validateRepos(cfg.Repos); err != nil {
		return err
	}
	if err := validateTools(cfg.Tools); err != nil {
		return err
	}
	if err := validateAliases(cfg.Shell.Aliases); err != nil {
		return err
	}
	if err := validateFunctions(cfg.Shell.Functions); err != nil {
		return err
	}
	if err := validateBuilds(cfg.Hooks.Builds); err != nil {
		return err
	}
	if err := validatePackages(cfg.Packages); err != nil {
		return err
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
	if err := validateDotfiles(cfg.Dotfiles); err != nil {
		return err
	}
	if err := validateDirectories(cfg.Directories); err != nil {
		return err
	}
	if err := validateRepos(cfg.Repos); err != nil {
		return err
	}
	if err := validateTools(cfg.Tools); err != nil {
		return err
	}
	if err := validateAliases(cfg.Shell.Aliases); err != nil {
		return err
	}
	if err := validateFunctions(cfg.Shell.Functions); err != nil {
		return err
	}
	if err := validateBuilds(cfg.Hooks.Builds); err != nil {
		return err
	}
	if err := validatePackages(cfg.Packages); err != nil {
		return err
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
