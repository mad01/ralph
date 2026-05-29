package config

// Config represents the main configuration structure for ralph.
// It will be loaded from a TOML file.
type Config struct {
	DotfilesRepoPath  string               `toml:"dotfiles_repo_path"`
	Dotfiles          map[string]Dotfile   `toml:"dotfiles"`
	DirsMirror        map[string]DirMirror `toml:"dirs_mirror"`
	Directories       map[string]Directory `toml:"directories"`
	Repos             map[string]Repo      `toml:"repos"`
	Tools             []Tool               `toml:"tools"`
	Shell             ShellConfig          `toml:"shell"`
	TemplateVariables map[string]any       `toml:"template_variables"`
	Hooks             HooksConfig          `toml:"hooks"`
	PackagesDir       string               `toml:"packages_dir"` // Default clone dir for remote pkgs (default: ~/.config/ralph/pkg)
	Packages          map[string]Package   `toml:"packages"`
	Recipes           []RecipeRef          `toml:"recipes"`        // Explicit recipe references (Mode A)
	RecipesConfig     RecipesConfig        `toml:"recipes_config"` // Auto-discovery configuration (Mode B)

	// loadedRecipes stores metadata about loaded recipes for migration support.
	// This is populated during config loading and not from the TOML file.
	LoadedRecipes []LoadedRecipeInfo `toml:"-"`

	// HostFilteredRecipes lists recipes that are enabled but skipped on this
	// host because of a host filter. Unlike disabled recipes (which should be
	// cleaned up), these belong to other hosts and their previously-recorded
	// artifacts must be frozen, not treated as orphans. Populated during
	// recipe loading; not read from the TOML file.
	HostFilteredRecipes []string `toml:"-"`
}

// LoadedRecipeInfo stores information about a loaded recipe for migration support.
type LoadedRecipeInfo struct {
	Path           string            // Path to the recipe file relative to dotfiles_repo_path
	Dir            string            // Directory containing the recipe (relative to dotfiles_repo_path)
	Name           string            // Recipe name from metadata
	LegacyPaths    map[string]string // Legacy path mappings for migration
	DeleteBehavior string            // "delete" (default) or "abandon"; controls cleanup of orphaned artifacts
	Wave           int               // Effective wave number (always >= 1 after ProcessRecipes)
	Caveats        string            // Post-apply instructions shown when a package in this recipe is rebuilt
	PreUninstall   []string          // Recipe-level pre_uninstall hooks, persisted into the manifest so cleanup can run them
	PostUninstall  []string          // Recipe-level post_uninstall hooks, persisted into the manifest so cleanup can run them
}

// DirMirror represents a directory whose contents should be mirrored into a
// target directory via symlinks. Each entry (file or subdirectory) in source
// becomes a symlink in target.
type DirMirror struct {
	Source      string   `toml:"source"`           // Relative path within the dotfiles_repo_path (resolved via ResolveRecipePaths)
	Target      string   `toml:"target"`           // Absolute path on the system, supporting ~
	Action      string   `toml:"action,omitempty"` // "symlink" (default) or "symlink_dir"
	Hosts       []string `toml:"hosts,omitempty"`  // List of hostnames this mirror should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                // Name of the recipe that defined this item; populated during merge
}

// Dotfile represents a single dotfile to be managed.
// The map key in Config.Dotfiles will be a logical name for the dotfile (e.g., "bashrc", "nvim_config").
type Dotfile struct {
	Source      string   `toml:"source"`                // Relative path within the dotfiles_repo_path
	Target      string   `toml:"target"`                // Absolute path on the system, supporting ~
	IsTemplate  bool     `toml:"is_template,omitempty"` // Whether this dotfile should be processed as a Go template
	Action      string   `toml:"action,omitempty"`      // "symlink" (default), "copy", or "symlink_dir"
	Hosts       []string `toml:"hosts,omitempty"`       // List of hostnames this dotfile should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"`      // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                     // Name of the recipe that defined this item; populated during merge
}

// Directory represents a directory to create.
type Directory struct {
	Target      string   `toml:"target"`           // Absolute path on the system, supporting ~
	Mode        string   `toml:"mode,omitempty"`   // Permission mode, e.g. "0755" (default)
	Hosts       []string `toml:"hosts,omitempty"`  // List of hostnames this directory should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                // Name of the recipe that defined this item; populated during merge
}

// Repo represents a git repository to clone.
type Repo struct {
	URL         string   `toml:"url"`              // Git repository URL
	Target      string   `toml:"target"`           // Absolute path on the system, supporting ~
	Branch      string   `toml:"branch,omitempty"` // Branch to checkout (optional)
	Commit      string   `toml:"commit,omitempty"` // Pin to specific commit (optional)
	Update      bool     `toml:"update,omitempty"` // Pull latest on each apply (optional)
	Hosts       []string `toml:"hosts,omitempty"`  // List of hostnames this repo should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                // Name of the recipe that defined this item; populated during merge
}

// Tool represents a standard tool that ralph can manage or check.
type Tool struct {
	Name         string    `toml:"name"`
	CheckCommand string    `toml:"check_command"`
	InstallHint  string    `toml:"install_hint"`
	ConfigFiles  []Dotfile `toml:"config_files,omitempty"` // Optional: config files for this tool
	Hosts        []string  `toml:"hosts,omitempty"`        // List of hostnames this tool should apply to (empty = all hosts)
	Enable       *bool     `toml:"enable,omitempty"`       // nil/true = enabled, false = disabled
	OwnerRecipe  string    `toml:"-"`                      // Name of the recipe that defined this item; populated during merge
}

// ShellConfig holds configurations related to shell aliases and functions.
type ShellConfig struct {
	Name      string                   `toml:"name,omitempty"` // Explicit shell name (bash/zsh/fish); auto-detected from $SHELL if omitted
	Aliases   map[string]ShellAlias    `toml:"aliases"`
	Functions map[string]ShellFunction `toml:"functions"`
	Env       map[string]string        `toml:"env"` // Environment variables (no host filtering for now)
	EnvOwners map[string]string        `toml:"-"`   // Tracks which recipe owns each env var; populated during merge
}

// ShellAlias represents a shell alias with optional host filtering.
type ShellAlias struct {
	Command     string   `toml:"command"`          // The command this alias executes
	Hosts       []string `toml:"hosts,omitempty"`  // List of hostnames this alias should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                // Name of the recipe that defined this item; populated during merge
}

// ShellFunction represents a custom shell function.
// The map key in ShellConfig.Functions will be the function name.
type ShellFunction struct {
	Body        string   `toml:"body"`             // The actual shell script for the function body
	Hosts       []string `toml:"hosts,omitempty"`  // List of hostnames this function should apply to (empty = all hosts)
	Enable      *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	OwnerRecipe string   `toml:"-"`                // Name of the recipe that defined this item; populated during merge
}

// HooksConfig holds configuration for various lifecycle hooks
type HooksConfig struct {
	PreApply      []string            `toml:"pre_apply"`      // Hooks to run before applying any dotfiles
	PostApply     []string            `toml:"post_apply"`     // Hooks to run after applying all dotfiles
	PreLink       map[string][]string `toml:"pre_link"`       // Hooks to run before linking a specific dotfile
	PostLink      map[string][]string `toml:"post_link"`      // Hooks to run after linking a specific dotfile
	Builds        map[string]Build    `toml:"builds"`         // Build hooks that run during apply
	PreUninstall  []string            `toml:"pre_uninstall"`  // Hooks to run during cleanup, before a removed/disabled recipe's artifacts are removed
	PostUninstall []string            `toml:"post_uninstall"` // Hooks to run during cleanup, after a removed/disabled recipe's artifacts are removed
}

// Build represents a build hook with multiple commands
type Build struct {
	Commands     []string `toml:"commands"`                // Commands to execute (mutually exclusive with Script)
	Script       string   `toml:"script,omitempty"`        // Path to a script to execute (mutually exclusive with Commands)
	WorkingDir   string   `toml:"working_dir,omitempty"`   // Working directory for commands
	Run          string   `toml:"run"`                     // "always", "once", or "manual"
	DependsOn    []string `toml:"depends_on,omitempty"`    // Dependencies: "builds.<name>" or "packages.<name>"
	Idempotent   bool     `toml:"idempotent,omitempty"`    // Skip when commands+working_dir hash matches last successful run
	InstallPaths []string `toml:"install_paths,omitempty"` // Declarative artifact list for cleanup tracking (no globs; HOME-prefixed)
	Hosts        []string `toml:"hosts,omitempty"`         // List of hostnames this build should apply to (empty = all hosts)
	Enable       *bool    `toml:"enable,omitempty"`        // nil/true = enabled, false = disabled
	Timeout      int      `toml:"timeout,omitempty"`       // Timeout in seconds (0 = default 600s)
	OwnerRecipe  string   `toml:"-"`                       // Name of the recipe that defined this item; populated during merge
	Wave         int      `toml:"-"`                       // Execution wave; populated during merge from RecipeMetadata.Wave
}

// RecipeRef represents a reference to a recipe file in the main config.
// Used for explicit [[recipes]] list mode.
type RecipeRef struct {
	Name   string   `toml:"name,omitempty"`   // Short name - looks for recipes/<name>/recipe.toml
	Path   string   `toml:"path,omitempty"`   // Full path to recipe.toml relative to dotfiles_repo_path
	Enable *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	Hosts  []string `toml:"hosts,omitempty"`  // List of hostnames this recipe should apply to (empty = all hosts)
}

// RecipeOverride provides enable/hosts overrides for auto-discovered recipes.
type RecipeOverride struct {
	Enable *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
	Hosts  []string `toml:"hosts,omitempty"`  // List of hostnames this recipe should apply to (empty = all hosts)
}

// RecipesConfig holds configuration for auto-discovery mode.
type RecipesConfig struct {
	AutoDiscover bool                      `toml:"auto_discover,omitempty"` // Enable auto-discovery of recipe.toml files
	AutoCleanup  bool                      `toml:"auto_cleanup,omitempty"`  // Run cleanup on every apply (no --enable-cleanup needed)
	Dir          string                    `toml:"dir,omitempty"`           // Directory to search for recipes (default: "recipes")
	Exclude      []string                  `toml:"exclude,omitempty"`       // Glob patterns to exclude from auto-discovery
	Overrides    map[string]RecipeOverride `toml:"overrides,omitempty"`     // Override enable/hosts for specific recipes by directory name
}

// DefaultRecipesDir is the default directory for recipes when using auto-discovery or short names.
const DefaultRecipesDir = "recipes"

// DefaultPackagesDir is the default directory for cloning remote packages.
const DefaultPackagesDir = "~/.config/ralph/pkg"

// DefaultClaudeSkillsDir is where Claude Code expects skills to be installed.
const DefaultClaudeSkillsDir = "~/.claude/skills"

// Package represents a managed package that can be updated and rebuilt.
type Package struct {
	Source       string   `toml:"source"`                  // "local", "remote", "make", or "go-install"
	Repo         string   `toml:"repo,omitempty"`          // Git URL (required for remote/make)
	Target       string   `toml:"target,omitempty"`        // Clone target (optional; defaults to <packages_dir>/<name>)
	Branch       string   `toml:"branch,omitempty"`        // Branch to track (remote only)
	WorkingDir   string   `toml:"working_dir,omitempty"`   // Dir for build/install (defaults to target for remote)
	Build        []string `toml:"build"`                   // Build commands
	Install      []string `toml:"install,omitempty"`       // Install commands (after build)
	Module       string   `toml:"module,omitempty"`        // Go module path (for go-install)
	Version      string   `toml:"version,omitempty"`       // Version tag (for go-install)
	DependsOn    []string `toml:"depends_on,omitempty"`    // Dependencies: "builds.<name>" or "packages.<name>"
	InstallPaths []string `toml:"install_paths,omitempty"` // Declarative artifact list for cleanup tracking (no globs; HOME-prefixed)
	Hosts        []string `toml:"hosts,omitempty"`         // Host filtering
	Enable       *bool    `toml:"enable,omitempty"`        // nil/true = enabled
	Timeout      int      `toml:"timeout,omitempty"`       // Timeout in seconds (0 = default 600s)
	OwnerRecipe  string   `toml:"-"`                       // Name of the recipe that defined this item; populated during merge
	Wave         int      `toml:"-"`                       // Execution wave; populated during merge from RecipeMetadata.Wave
}

// RecipeMetadata contains optional metadata about a recipe.
type RecipeMetadata struct {
	Name           string            `toml:"name,omitempty"`            // Human-readable name for the recipe
	Description    string            `toml:"description,omitempty"`     // Description of what this recipe provides
	LegacyPaths    map[string]string `toml:"legacy_paths,omitempty"`    // Map of old source paths to new paths for migration
	DeleteBehavior string            `toml:"delete_behavior,omitempty"` // "delete" (default) or "abandon" — how cleanup handles orphans when this recipe is removed
	Wave           *int              `toml:"wave,omitempty"`            // Execution wave: lower waves complete first (nil = unset → defaults to 1; wave 0 runs before default)
	Caveats        string            `toml:"caveats,omitempty"`         // Post-apply instructions shown when a package in this recipe is rebuilt
}

// DeleteBehaviorDelete instructs ralph to remove orphaned artifacts when a recipe is gone.
const DeleteBehaviorDelete = "delete"

// DeleteBehaviorAbandon instructs ralph to leave orphaned artifacts in place when a recipe is gone.
const DeleteBehaviorAbandon = "abandon"

// Recipe represents a modular configuration file (recipe.toml) that can be
// placed alongside source files in the dotfiles repository.
type Recipe struct {
	Recipe            RecipeMetadata       `toml:"recipe"`             // Metadata about this recipe
	Dotfiles          map[string]Dotfile   `toml:"dotfiles"`           // Dotfiles defined in this recipe
	DirsMirror        map[string]DirMirror `toml:"dirs_mirror"`        // Directory mirrors (bulk symlinking)
	Directories       map[string]Directory `toml:"directories"`        // Directories to create
	Repos             map[string]Repo      `toml:"repos"`              // Repos to clone
	Tools             []Tool               `toml:"tools"`              // Tools to check/manage
	Shell             ShellConfig          `toml:"shell"`              // Shell configuration (aliases, functions, env)
	Hooks             HooksConfig          `toml:"hooks"`              // Hooks (pre/post apply, builds)
	Packages          map[string]Package   `toml:"packages"`           // Managed packages
	TemplateVariables map[string]any       `toml:"template_variables"` // Template variables
}
