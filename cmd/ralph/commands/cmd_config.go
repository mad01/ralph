package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/mad01/ralph/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Show the merged user-editable config: the main config file with the
config.local.toml overlay applied, before recipes are merged in. This is
the state your files produce — what you would edit. For the fully
resolved state after recipe processing, use 'ralph doctor' and
'ralph state show'.

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
  hosts = ["yesyes"]           # optional host gate (empty = all hosts)
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

The full key-by-key reference lives in docs/configuration.md in the
ralph repo.

Pair it with doctor: doctor shows the state ralph resolved, config shows
which file and key to change.`,
	Args: cobra.NoArgs,
	RunE: runConfigCmd,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

// configState is what the config command resolved: the config file paths, the
// load status, and (when loading succeeded) the merged user config keyed the
// way the TOML file spells it.
type configState struct {
	ConfigFile   string         `json:"config_file"`
	Status       string         `json:"status"`
	LocalOverlay string         `json:"local_overlay"`
	LocalPresent bool           `json:"local_present"`
	Config       map[string]any `json:"config"`
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
	}
	var cfg *config.Config
	if _, statErr := os.Stat(mainPath); os.IsNotExist(statErr) {
		st.Status = "missing, run 'ralph init' to create one"
	} else if cfg, st.LocalPresent, err = config.LoadUserConfig(mainPath); err != nil {
		st.Status = "parse error: " + err.Error()
		cfg = nil
	}

	if cfg != nil {
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
// merged user config as TOML.
func printConfigText(st configState) {
	fmt.Printf("config file:   %s (%s)\n", st.ConfigFile, st.Status)
	overlay := "absent"
	if st.LocalPresent {
		overlay = "present"
	}
	fmt.Printf("local overlay: %s (%s)\n", st.LocalOverlay, overlay)
	if st.Config == nil {
		return
	}
	fmt.Println()
	if err := toml.NewEncoder(os.Stdout).Encode(st.Config); err != nil {
		// The map already round-tripped through the encoder once, so this is a
		// broken invariant, not an expected error.
		panic("config: re-encoding config map: " + err.Error())
	}
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
