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

// isValidShellIdentifier checks that a name is a valid POSIX shell identifier:
// non-empty, starts with a letter or underscore, contains only letters, digits,
// and underscores.
func isValidShellIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' {
			continue
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// isValidAliasName checks that a name is valid for a shell alias.
// Aliases are more permissive than identifiers — they allow dots, dashes,
// colons, at-signs, slashes, plus, and percent (e.g. "....", "docker-compose").
// Rejected: empty names, names containing shell metacharacters (; | & $ ` \ " ' ( ) { } < > space tab newline # ! ~ * ? [ ]).
func isValidAliasName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch r {
		case ';', '|', '&', '$', '`', '\\', '"', '\'', '(', ')', '{', '}', '<', '>', ' ', '\t', '\n', '#', '!', '~', '*', '?', '[', ']':
			return false
		}
	}
	return true
}

// validateAliases validates a map of ShellAlias entries.
func validateAliases(aliases map[string]ShellAlias) error {
	for aliasName, alias := range aliases {
		if !isValidAliasName(aliasName) {
			return fmt.Errorf("shell alias '%s': name contains invalid characters (shell metacharacters are not allowed)", aliasName)
		}
		if alias.Command == "" {
			return fmt.Errorf("shell alias '%s': command cannot be empty", aliasName)
		}
	}
	return nil
}

// isValidFunctionName checks that a name is valid for a shell function.
// Bash and zsh allow dashes and dots in function names (e.g. "apply-system-kitty-theme").
// Same metacharacter blocklist as aliases.
func isValidFunctionName(name string) bool {
	return isValidAliasName(name)
}

// validateFunctions validates a map of ShellFunction entries.
func validateFunctions(funcs map[string]ShellFunction) error {
	for funcName, shellFunc := range funcs {
		if !isValidFunctionName(funcName) {
			return fmt.Errorf("shell function '%s': name contains invalid characters (shell metacharacters are not allowed)", funcName)
		}
		if shellFunc.Body == "" {
			return fmt.Errorf("shell function '%s': body cannot be empty", funcName)
		}
	}
	return nil
}

// validateEnvVars validates shell environment variable names.
func validateEnvVars(env map[string]string) error {
	for name := range env {
		if !isValidShellIdentifier(name) {
			return fmt.Errorf("shell env var '%s': name must be a valid shell identifier (letters, digits, underscores; cannot start with a digit)", name)
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
		if build.Timeout < 0 {
			return fmt.Errorf("build '%s': timeout must be non-negative, got %d", name, build.Timeout)
		}
	}
	return nil
}

// validatePackages validates a map of Package entries.
func validatePackages(pkgs map[string]Package) error {
	for name, pkg := range pkgs {
		if pkg.Source == "" {
			return fmt.Errorf("package '%s': source is required (local, remote, or make)", name)
		}
		if pkg.Source != "local" && pkg.Source != "remote" && pkg.Source != "make" {
			return fmt.Errorf("package '%s': source must be 'local', 'remote', or 'make', got '%s'", name, pkg.Source)
		}
		if (pkg.Source == "remote" || pkg.Source == "make") && pkg.Repo == "" {
			return fmt.Errorf("package '%s': repo is required for remote packages", name)
		}
		if pkg.Source == "local" && pkg.WorkingDir == "" {
			return fmt.Errorf("package '%s': working_dir is required for local packages", name)
		}
		// source=make has implicit defaults for build/install, so build is only required for local/remote
		if pkg.Source != "make" && len(pkg.Build) == 0 {
			return fmt.Errorf("package '%s': at least one build command is required", name)
		}
		if pkg.Timeout < 0 {
			return fmt.Errorf("package '%s': timeout must be non-negative, got %d", name, pkg.Timeout)
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
	if err := validateEnvVars(cfg.Shell.Env); err != nil {
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
	if err := validateEnvVars(cfg.Shell.Env); err != nil {
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
