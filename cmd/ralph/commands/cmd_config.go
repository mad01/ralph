package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Show the merged user-editable config: the main config file with the
config.local.toml overlay applied, before recipes are merged in. This is
the state your files produce — what you would edit.

With --effective, show the fully resolved config instead: recipes merged
in, host and profile gates applied, defaults materialized — the config
every other ralph command runs on. On a machine fed by [[recipe_sources]]
the two differ substantially: the user files stay small while the
effective config carries every recipe item. The effective output ends
with the loaded-recipe list (with waves) and the host-filtered recipes
whose artifacts ralph freezes rather than cleans up.

Main config resolution order:

  1. $RALPH_CONFIG (when set, the file must exist)
  2. $XDG_CONFIG_HOME/ralph/config.toml
  3. ~/.config/ralph/config.toml

A git-ignored config.local.toml next to the main file carries
machine-local overrides. Merge semantics: local always wins — scalar
values override when set, maps merge per key, lists replace the whole
list. Profiles normally live only in the overlay.

Annotated example (all top-level keys):

  dotfiles_repo_path = "~/dotfiles"     # repo that relative source paths resolve against
  packages_dir = "~/.config/ralph/pkg"  # clone dir for remote packages (default shown)
  profiles = ["personal"]               # machine profile labels; usually set in config.local.toml

  [dotfiles.gitconfig]         # a file to place: symlink (default), copy, or symlink_dir
  source = "git/.gitconfig"    # relative to dotfiles_repo_path
  target = "~/.gitconfig"      # absolute target, ~ expands
  is_template = false          # process as a Go template with template_variables
  hosts = ["work-laptop"]      # optional host gate (empty = all hosts)
  profiles = ["personal"]      # optional profile gate (empty = all profiles)
  enable = true                # optional; false disables the item

  [dirs_mirror.bin]            # mirror a directory's entries as individual symlinks
  source = "bin"
  target = "~/bin"

  [directories.workspace]      # a directory to create
  target = "~/code"
  mode = "0755"                # permission mode (default "0755")

  [repos.dotfiles]             # a git repo to clone
  url = "git@github.com:me/dotfiles.git"
  target = "~/dotfiles"
  update = true                # pull on each apply; branch/commit pin also available

  [[tools]]                    # a tool to check for (doctor runs check_command)
  name = "fzf"
  check_command = "command -v fzf"
  install_hint = "brew install fzf"

  [shell.aliases.k]            # aliases/functions/env written into the shell rc block
  command = "kubectl"

  [template_variables]         # values for {{ .name }} in is_template dotfiles
  git_email = "me@example.com"

  [hooks]                      # lifecycle hooks; pre_link/post_link exist per dotfile
  post_apply = ["echo done"]

  [hooks.builds.mytool]        # build hooks run during apply
  commands = ["make install"]
  working_dir = "~/src/mytool"
  run = "once"                 # "always", "once", or "manual"
  verify = "mytool version"    # doctor runs this; non-zero exit = drift

  [packages.csl]               # packages ralph clones, builds, installs, and tracks
  source = "remote"            # "local", "remote", "make", or "go-install"
  repo = "https://github.com/mad01/thismoon.git"
  build = ["make -C services/csl build"]
  install = ["make -C services/csl install"]

  [[recipes]]                  # explicit recipe references (Mode A)
  name = "nvim"                # loads recipes/nvim/recipe.toml

  [recipes_config]             # recipe auto-discovery (Mode B) + per-recipe overrides
  auto_discover = true

  [[recipe_sources]]           # remote recipe repos cached under ~/.config/ralph/sources
  name = "thismoon"
  url = "https://github.com/mad01/thismoon.git"
  ref = "main"
  update = true
  profiles = ["personal"]      # skip checkout and sync on non-matching machines

The full key-by-key reference lives in docs/configuration.md in the
ralph repo.

Pair it with doctor: doctor shows the state ralph resolved, config shows
which file and key to change.`,
	Args: cobra.NoArgs,
	RunE: runConfigCmd,
}

var configEffective bool

func init() {
	configCmd.Flags().BoolVar(&configEffective, "effective", false,
		"show the fully resolved config: recipes merged, host/profile gates applied")
	rootCmd.AddCommand(configCmd)
}

// configState is what the config command resolved: the config file paths, the
// load status, and (when loading succeeded) the merged config keyed the way
// the TOML file spells it. The recipe fields are populated only in effective
// mode — recipe processing is what fills them.
type configState struct {
	ConfigFile          string         `json:"config_file"`
	Status              string         `json:"status"`
	LocalOverlay        string         `json:"local_overlay"`
	LocalPresent        bool           `json:"local_present"`
	Effective           bool           `json:"effective"`
	Config              map[string]any `json:"config"`
	LoadedRecipes       []loadedRecipe `json:"loaded_recipes,omitempty"`
	HostFilteredRecipes []string       `json:"host_filtered_recipes,omitempty"`
}

// loadedRecipe is the provenance a loaded recipe contributes to the config
// output: which recipe, at which wave.
type loadedRecipe struct {
	Name string `json:"name"`
	Wave int    `json:"wave"`
}

func runConfigCmd(cmd *cobra.Command, args []string) error {
	mainPath, err := config.GetDefaultConfigPath()
	if err != nil {
		return err
	}

	st := configState{
		ConfigFile:   mainPath,
		Status:       "loaded",
		LocalOverlay: config.LocalConfigPath(mainPath),
		Effective:    configEffective,
	}
	var cfg *config.Config
	switch _, statErr := os.Stat(mainPath); {
	case os.IsNotExist(statErr):
		st.Status = "missing, run 'ralph init' to create one"
	case configEffective:
		// The overlay merges inside LoadConfigWithHost, so its presence is
		// re-derived from the file rather than returned by the loader.
		if cfg, err = config.LoadConfigWithHost(""); err != nil {
			st.Status = "error: " + err.Error()
			cfg = nil
		} else if _, overlayErr := os.Stat(st.LocalOverlay); overlayErr == nil {
			st.LocalPresent = true
		}
	default:
		if cfg, st.LocalPresent, err = config.LoadUserConfig(mainPath); err != nil {
			st.Status = "parse error: " + err.Error()
			cfg = nil
		}
	}

	if cfg != nil {
		materializeDefaults(cfg)
		for _, r := range cfg.LoadedRecipes {
			st.LoadedRecipes = append(st.LoadedRecipes, loadedRecipe{Name: r.Name, Wave: r.Wave})
		}
		sortLoadedRecipes(st.LoadedRecipes)
		st.HostFilteredRecipes = cfg.HostFilteredRecipes
		if st.Config, err = configAsMap(cfg); err != nil {
			return err
		}
	}

	if outputJSON() {
		b, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding config state: %w", err)
		}
		fmt.Println(string(b))
	} else {
		printConfigText(st)
	}

	if cfg == nil {
		return &ExitError{Code: 1}
	}
	return nil
}

// printConfigText prints the header lines and, when the config loaded, the
// merged config as TOML. The effective-mode recipe provenance prints as TOML
// comment lines so the whole output stays one parseable document.
func printConfigText(st configState) {
	fmt.Printf("config file:   %s (%s)\n", st.ConfigFile, st.Status)
	overlay := "absent"
	if st.LocalPresent {
		overlay = "present"
	}
	fmt.Printf("local overlay: %s (%s)\n", st.LocalOverlay, overlay)
	if sourcesDir, err := config.SourcesDir(); err == nil {
		fmt.Printf("sources dir:   %s\n", sourcesDir)
	}
	if st.Effective {
		fmt.Println("mode:          effective (recipes merged, host/profile gates applied)")
	}
	if st.Config == nil {
		return
	}
	fmt.Println()
	if err := toml.NewEncoder(os.Stdout).Encode(st.Config); err != nil {
		// The map already round-tripped through the encoder once, so this is a
		// broken invariant, not an expected error.
		panic("config: re-encoding config map: " + err.Error())
	}
	printRecipeProvenance(st)
}

// printRecipeProvenance appends the effective-mode recipe sections: which
// recipes contributed items (with waves), and which are frozen by a host or
// profile gate — the set whose artifacts cleanup must not treat as orphans.
func printRecipeProvenance(st configState) {
	if !st.Effective {
		return
	}
	fmt.Println()
	fmt.Printf("# loaded recipes (%d):\n", len(st.LoadedRecipes))
	for _, r := range st.LoadedRecipes {
		fmt.Printf("#   wave %d  %s\n", r.Wave, r.Name)
	}
	if len(st.HostFilteredRecipes) > 0 {
		fmt.Printf("# host-filtered recipes (%d, artifacts frozen): %s\n",
			len(st.HostFilteredRecipes), strings.Join(st.HostFilteredRecipes, ", "))
	}
}

// materializeDefaults pins the directory defaults ralph resolves at the point
// of use, so the printed value is the value ralph will actually use rather
// than an empty string.
func materializeDefaults(cfg *config.Config) {
	if cfg.PackagesDir == "" {
		cfg.PackagesDir = config.DefaultPackagesDir
	}
	if cfg.RecipesConfig.Dir == "" {
		cfg.RecipesConfig.Dir = config.DefaultRecipesDir
	}
}

// sortLoadedRecipes orders recipes by wave, then name, matching the order
// apply executes them in.
func sortLoadedRecipes(recipes []loadedRecipe) {
	sort.Slice(recipes, func(i, j int) bool {
		if recipes[i].Wave != recipes[j].Wave {
			return recipes[i].Wave < recipes[j].Wave
		}
		return recipes[i].Name < recipes[j].Name
	})
}

// configAsMap round-trips cfg through its TOML encoding so every output format
// carries the key names the config file spells, not Go field names.
func configAsMap(cfg *config.Config) (map[string]any, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	var m map[string]any
	if _, err := toml.Decode(buf.String(), &m); err != nil {
		return nil, fmt.Errorf("re-decoding config: %w", err)
	}
	return m, nil
}
