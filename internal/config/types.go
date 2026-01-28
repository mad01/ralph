package config

// Config represents the main configuration structure for dotter.
// It will be loaded from a TOML file.
type Config struct {
	DotfilesRepoPath  string                 `toml:"dotfiles_repo_path"`
	Dotfiles          map[string]Dotfile     `toml:"dotfiles"`
	Directories       map[string]Directory   `toml:"directories"`
	Repos             map[string]Repo        `toml:"repos"`
	Tools             []Tool                 `toml:"tools"`
	Shell             ShellConfig            `toml:"shell"`
	TemplateVariables map[string]interface{} `toml:"template_variables"`
	Hooks             HooksConfig            `toml:"hooks"`
	Recipes           []RecipeRef            `toml:"recipes"`        // Explicit recipe references (Mode A)
	RecipesConfig     RecipesConfig          `toml:"recipes_config"` // Auto-discovery configuration (Mode B)

	// loadedRecipes stores metadata about loaded recipes for migration support.
	// This is populated during config loading and not from the TOML file.
	LoadedRecipes []LoadedRecipeInfo `toml:"-"`
}

// LoadedRecipeInfo stores information about a loaded recipe for migration support.
type LoadedRecipeInfo struct {
	Path        string            // Path to the recipe file relative to dotfiles_repo_path
	Dir         string            // Directory containing the recipe (relative to dotfiles_repo_path)
	Name        string            // Recipe name from metadata
	LegacyPaths map[string]string // Legacy path mappings for migration
}

// Dotfile represents a single dotfile to be managed.
// The map key in Config.Dotfiles will be a logical name for the dotfile (e.g., "bashrc", "nvim_config").
type Dotfile struct {
	Source     string   `toml:"source"`                // Relative path within the dotfiles_repo_path
	Target     string   `toml:"target"`                // Absolute path on the system, supporting ~
	IsTemplate bool     `toml:"is_template,omitempty"` // Whether this dotfile should be processed as a Go template
	Action     string   `toml:"action,omitempty"`      // "symlink" (default), "copy", or "symlink_dir"
	Hosts      []string `toml:"hosts,omitempty"`       // List of hostnames this dotfile should apply to (empty = all hosts)
	Enable     *bool    `toml:"enable,omitempty"`      // nil/true = enabled, false = disabled
}

// Directory represents a directory to create.
type Directory struct {
	Target string   `toml:"target"`          // Absolute path on the system, supporting ~
	Mode   string   `toml:"mode,omitempty"`  // Permission mode, e.g. "0755" (default)
	Hosts  []string `toml:"hosts,omitempty"` // List of hostnames this directory should apply to (empty = all hosts)
	Enable *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
}

// Repo represents a git repository to clone.
type Repo struct {
	URL    string   `toml:"url"`              // Git repository URL
	Target string   `toml:"target"`           // Absolute path on the system, supporting ~
	Branch string   `toml:"branch,omitempty"` // Branch to checkout (optional)
	Commit string   `toml:"commit,omitempty"` // Pin to specific commit (optional)
	Update bool     `toml:"update,omitempty"` // Pull latest on each apply (optional)
	Hosts  []string `toml:"hosts,omitempty"`  // List of hostnames this repo should apply to (empty = all hosts)
	Enable *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
}

// Tool represents a standard tool that dotter can manage or check.
type Tool struct {
	Name         string    `toml:"name"`
	CheckCommand string    `toml:"check_command"`
	InstallHint  string    `toml:"install_hint"`
	ConfigFiles  []Dotfile `toml:"config_files,omitempty"` // Optional: config files for this tool
	Hosts        []string  `toml:"hosts,omitempty"`        // List of hostnames this tool should apply to (empty = all hosts)
	Enable       *bool     `toml:"enable,omitempty"`       // nil/true = enabled, false = disabled
}

// ShellConfig holds configurations related to shell aliases and functions.
type ShellConfig struct {
	Aliases   map[string]ShellAlias    `toml:"aliases"`
	Functions map[string]ShellFunction `toml:"functions"`
	Env       map[string]string        `toml:"env"` // Environment variables (no host filtering for now)
}

// ShellAlias represents a shell alias with optional host filtering.
type ShellAlias struct {
	Command string   `toml:"command"`         // The command this alias executes
	Hosts   []string `toml:"hosts,omitempty"` // List of hostnames this alias should apply to (empty = all hosts)
	Enable  *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
}

// ShellFunction represents a custom shell function.
// The map key in ShellConfig.Functions will be the function name.
type ShellFunction struct {
	Body   string   `toml:"body"`            // The actual shell script for the function body
	Hosts  []string `toml:"hosts,omitempty"` // List of hostnames this function should apply to (empty = all hosts)
	Enable *bool    `toml:"enable,omitempty"` // nil/true = enabled, false = disabled
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
	Hosts      []string `toml:"hosts,omitempty"`       // List of hostnames this build should apply to (empty = all hosts)
	Enable     *bool    `toml:"enable,omitempty"`      // nil/true = enabled, false = disabled
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
	Dir          string                    `toml:"dir,omitempty"`           // Directory to search for recipes (default: "recipes")
	Exclude      []string                  `toml:"exclude,omitempty"`       // Glob patterns to exclude from auto-discovery
	Overrides    map[string]RecipeOverride `toml:"overrides,omitempty"`     // Override enable/hosts for specific recipes by directory name
}

// DefaultRecipesDir is the default directory for recipes when using auto-discovery or short names.
const DefaultRecipesDir = "recipes"

// RecipeMetadata contains optional metadata about a recipe.
type RecipeMetadata struct {
	Name        string            `toml:"name,omitempty"`         // Human-readable name for the recipe
	Description string            `toml:"description,omitempty"`  // Description of what this recipe provides
	LegacyPaths map[string]string `toml:"legacy_paths,omitempty"` // Map of old source paths to new paths for migration
}

// Recipe represents a modular configuration file (recipe.toml) that can be
// placed alongside source files in the dotfiles repository.
type Recipe struct {
	Recipe            RecipeMetadata         `toml:"recipe"`             // Metadata about this recipe
	Dotfiles          map[string]Dotfile     `toml:"dotfiles"`           // Dotfiles defined in this recipe
	Directories       map[string]Directory   `toml:"directories"`        // Directories to create
	Repos             map[string]Repo        `toml:"repos"`              // Repos to clone
	Tools             []Tool                 `toml:"tools"`              // Tools to check/manage
	Shell             ShellConfig            `toml:"shell"`              // Shell configuration (aliases, functions, env)
	Hooks             HooksConfig            `toml:"hooks"`              // Hooks (pre/post apply, builds)
	TemplateVariables map[string]interface{} `toml:"template_variables"` // Template variables
}
