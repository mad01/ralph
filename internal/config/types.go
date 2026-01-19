package config

// Config represents the main configuration structure for dotter.
// It will be loaded from a TOML file.
type Config struct {
	DotfilesRepoPath  string                 `toml:"dotfiles_repo_path"`
	Dotfiles          map[string]Dotfile     `toml:"dotfiles"`
	Tools             []Tool                 `toml:"tools"`
	Shell             ShellConfig            `toml:"shell"`
	TemplateVariables map[string]interface{} `toml:"template_variables"`
	Hooks             HooksConfig            `toml:"hooks"`
}

// Dotfile represents a single dotfile to be managed.
// The map key in Config.Dotfiles will be a logical name for the dotfile (e.g., "bashrc", "nvim_config").
type Dotfile struct {
	Source     string `toml:"source"`                // Relative path within the dotfiles_repo_path
	Target     string `toml:"target"`                // Absolute path on the system, supporting ~
	IsTemplate bool   `toml:"is_template,omitempty"` // Whether this dotfile should be processed as a Go template
	Action     string `toml:"action,omitempty"`      // "symlink" (default) or "copy"
}

// Tool represents a standard tool that dotter can manage or check.
type Tool struct {
	Name         string    `toml:"name"`
	CheckCommand string    `toml:"check_command"`
	InstallHint  string    `toml:"install_hint"`
	ConfigFiles  []Dotfile `toml:"config_files,omitempty"` // Optional: config files for this tool
}

// ShellConfig holds configurations related to shell aliases and functions.
type ShellConfig struct {
	Aliases   map[string]string        `toml:"aliases"`
	Functions map[string]ShellFunction `toml:"functions"`
}

// ShellFunction represents a custom shell function.
// The map key in ShellConfig.Functions will be the function name.
type ShellFunction struct {
	Body string `toml:"body"` // The actual shell script for the function body
}

// HooksConfig holds configuration for various lifecycle hooks
type HooksConfig struct {
	PreApply  []string            `toml:"pre_apply"`  // Hooks to run before applying any dotfiles
	PostApply []string            `toml:"post_apply"` // Hooks to run after applying all dotfiles
	PreLink   map[string][]string `toml:"pre_link"`   // Hooks to run before linking a specific dotfile
	PostLink  map[string][]string `toml:"post_link"`  // Hooks to run after linking a specific dotfile
	Builds    map[string]Build    `toml:"builds"`     // Build hooks that run during apply
}

// Build represents a build hook with multiple commands
type Build struct {
	Commands   []string `toml:"commands"`              // Commands to execute
	WorkingDir string   `toml:"working_dir,omitempty"` // Working directory for commands
	Run        string   `toml:"run"`                   // "always", "once", or "manual"
}
