# Configuration Reference

Ralph uses a single TOML configuration file to define how your dotfiles, shell environment, repositories, packages, and hooks are managed.

## Config File Location

Ralph looks for its configuration at:

```
$XDG_CONFIG_HOME/ralph/config.toml
```

If `$XDG_CONFIG_HOME` is not set, it defaults to:

```
~/.config/ralph/config.toml
```

Run `ralph init` to create a starter config interactively. See [commands](commands.md) for details.

## Top-Level Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `dotfiles_repo_path` | string | yes | -- | Absolute path to your dotfiles source repository. Supports `~`. |
| `packages_dir` | string | no | `~/.config/ralph/pkg` | Directory where remote packages are cloned. |

## Common Field Patterns

Most configuration sections share two optional fields for controlling when items are applied:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enable` | bool (pointer) | `nil` (enabled) | `nil` or `true` means enabled; `false` means disabled. |
| `hosts` | string array | `[]` (all hosts) | Apply only on the listed hostnames. Empty means all hosts. |

## Sections

### `[dotfiles.<name>]`

Defines a dotfile to symlink, copy, or template into place. The map key is a logical name (e.g., `bashrc`, `nvim_config`).

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `source` | string | yes | -- | Path to the source file, relative to `dotfiles_repo_path`. |
| `target` | string | yes | -- | Absolute destination path on the system. Supports `~`. |
| `is_template` | bool | no | `false` | Process the source as a Go template before linking. |
| `action` | string | no | `"symlink"` | One of `"symlink"`, `"copy"`, or `"symlink_dir"`. |
| `hosts` | string array | no | `[]` | Host filtering (empty = all hosts). |
| `enable` | bool (pointer) | no | `nil` | `nil`/`true` = enabled, `false` = disabled. |

```toml
[dotfiles.bashrc]
source = "bash/.bashrc"
target = "~/.bashrc"

[dotfiles.nvim]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.gitconfig]
source = "git/gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true
```

### `[directories.<name>]`

Defines a directory to create if it does not exist.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `target` | string | yes | -- | Absolute path of the directory to create. Supports `~`. |
| `mode` | string | no | `"0755"` | Permission mode for the directory. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[directories.screenshots]
target = "~/Screenshots"

[directories.local_bin]
target = "~/.local/bin"
mode = "0700"
```

### `[repos.<name>]`

Defines a git repository to clone and optionally keep updated.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | Git repository URL. |
| `target` | string | yes | -- | Local clone path. Supports `~`. |
| `branch` | string | no | -- | Branch to checkout after cloning. |
| `commit` | string | no | -- | Pin to a specific commit hash. |
| `update` | bool | no | `false` | Pull latest changes on each `ralph apply`. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[repos.tmux-plugins]
url = "https://github.com/tmux-plugins/tpm"
target = "~/.tmux/plugins/tpm"
update = true
```

### `[[tools]]`

Defines external tools to check for during `apply` and `doctor`. Tools are not installed by ralph -- the `install_hint` is shown when a tool is missing.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | -- | Display name for the tool. |
| `check_command` | string | yes | -- | Shell command to check if the tool is installed (exit 0 = installed). |
| `install_hint` | string | yes | -- | Human-readable install instructions shown when the tool is missing. |
| `config_files` | array of Dotfile | no | `[]` | Optional dotfile entries for this tool's config files. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[[tools]]
name = "fzf"
check_command = "command -v fzf"
install_hint = "brew install fzf"

[[tools]]
name = "ripgrep"
check_command = "command -v rg"
install_hint = "brew install ripgrep"
hosts = ["work-laptop"]
```

### `[shell]`

Configures shell aliases, functions, and environment variables. Ralph generates shell-specific script files and injects `source` lines into your RC file.

| Field | Type | Required | Default | Description |
|-------|------|---------|---------|-------------|
| `name` | string | no | auto-detected from `$SHELL` | Explicit shell name: `"bash"`, `"zsh"`, or `"fish"`. |

#### `[shell.aliases.<name>]`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `command` | string | yes | -- | The command this alias executes. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[shell.aliases.ll]
command = "ls -alh"

[shell.aliases.k]
command = "kubectl"
hosts = ["work-laptop"]
```

#### `[shell.functions.<name>]`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `body` | string | yes | -- | The shell script body of the function. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[shell.functions.mkcd]
body = """
mkdir -p "$1" && cd "$1"
"""
```

#### `[shell.env]`

Simple key-value pairs for environment variables. These are exported in the generated shell scripts. Host filtering is not supported on individual env vars.

```toml
[shell.env]
EDITOR = "nvim"
GOPATH = "$HOME/go"
```

### `[template_variables]`

A key-value map of variables available in Go templates when `is_template = true` is set on a dotfile. Values can be strings, numbers, or booleans.

```toml
[template_variables]
email = "user@example.com"
font_size = 14
use_dark_theme = true
```

In a template file, access these as `{{ .TemplateVariables.email }}`.

### `[hooks]`

Lifecycle hooks that run shell commands at specific points during `ralph apply`.

| Field | Type | Description |
|-------|------|-------------|
| `pre_apply` | string array | Commands to run before any apply operations. |
| `post_apply` | string array | Commands to run after all apply operations. |

```toml
[hooks]
pre_apply = ["echo 'Starting apply...'"]
post_apply = ["echo 'Apply finished.'"]
```

#### `[hooks.pre_link]` and `[hooks.post_link]`

Maps of dotfile name to a list of commands. These run before/after linking a specific dotfile.

```toml
[hooks.pre_link]
nvim_config = ["echo 'About to link nvim config'"]

[hooks.post_link]
bashrc = ["source ~/.bashrc"]
```

#### `[hooks.builds.<name>]`

Build hooks run during `ralph apply` after dotfiles and shell configuration are processed.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `commands` | string array | yes | -- | Shell commands to execute in order. |
| `working_dir` | string | no | -- | Directory to run commands in. Supports `~`. |
| `run` | string | yes | -- | Run mode: `"always"`, `"once"`, or `"manual"`. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

Run modes:

- `"always"` -- runs on every `ralph apply`.
- `"once"` -- runs once and is skipped on subsequent applies (unless `--force` or `--reset-builds` is used).
- `"manual"` -- only runs when explicitly requested via `--build=NAME`.

```toml
[hooks.builds.my-tool]
commands = ["go build -o ~/.local/bin/mytool ."]
working_dir = "~/code/mytool"
run = "once"
```

### `[packages.<name>]`

Managed packages synced with `ralph sync` and built during `ralph apply`. Packages track build state separately from build hooks, using `pkg:` prefixed keys in the state file.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `source` | string | yes | -- | `"local"` or `"remote"`. |
| `repo` | string | remote only | -- | Git URL for remote packages. |
| `target` | string | no | `<packages_dir>/<name>` | Clone target directory for remote packages. |
| `branch` | string | no | -- | Branch to track (remote only). |
| `working_dir` | string | no | `target` (remote) | Directory to run build/install commands in. Supports `~`. |
| `build` | string array | yes | -- | Build commands to execute in order. |
| `install` | string array | no | `[]` | Install commands to run after a successful build. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[packages.neovim]
source = "remote"
repo = "https://github.com/neovim/neovim"
branch = "stable"
build = ["make CMAKE_BUILD_TYPE=Release"]
install = ["sudo make install"]

[packages.my-cli]
source = "local"
working_dir = "~/code/my-cli"
build = ["go build -o ~/.local/bin/my-cli ."]
```

### `[[recipes]]`

Explicit references to modular recipe files. Recipes are standalone `recipe.toml` files placed alongside source files in your dotfiles repository.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | no | -- | Short name. Resolves to `recipes/<name>/recipe.toml` relative to `dotfiles_repo_path`. |
| `path` | string | no | -- | Full path to the recipe file, relative to `dotfiles_repo_path`. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |
| `hosts` | string array | no | `[]` | Host filtering. |

Provide either `name` or `path`, not both.

```toml
[[recipes]]
name = "nvim"

[[recipes]]
path = "tools/tmux/recipe.toml"
hosts = ["work-laptop"]
```

### `[recipes_config]`

Controls automatic discovery of recipe files. When `auto_discover` is enabled, ralph searches for `recipe.toml` files under the configured directory.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `auto_discover` | bool | no | `false` | Enable automatic recipe discovery. |
| `dir` | string | no | `"recipes"` | Directory to search, relative to `dotfiles_repo_path`. |
| `exclude` | string array | no | `[]` | Glob patterns to exclude from auto-discovery. |

```toml
[recipes_config]
auto_discover = true
dir = "recipes"
exclude = ["experimental/*"]
```

#### `[recipes_config.overrides.<name>]`

Override `enable` and `hosts` for auto-discovered recipes by directory name.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |
| `hosts` | string array | no | `[]` | Host filtering. |

```toml
[recipes_config.overrides.work-tools]
hosts = ["work-laptop"]

[recipes_config.overrides.deprecated-stuff]
enable = false
```

### Recipe File Format (`recipe.toml`)

A recipe file can contain any of the same sections as the main config (except top-level fields and recipe references). It also supports metadata and legacy path mappings for migration.

```toml
[recipe]
name = "Neovim"
description = "Neovim editor configuration"

[recipe.legacy_paths]
"ralph_files/nvim/init.lua" = "nvim/init.lua"

[dotfiles.nvim_init]
source = "init.lua"
target = "~/.config/nvim/init.lua"

[shell.aliases.vim]
command = "nvim"
```

## Build State

Ralph tracks build completion in a JSON file at:

```
~/.config/ralph/.builds_state
```

- Build hooks use their name as the key (e.g., `"my-tool"`).
- Packages use a `pkg:` prefix (e.g., `"pkg:neovim"`).
- Each entry records the completion timestamp and the git hash at build time (when available).

Use `--reset-builds` on `ralph apply` to clear all build state, or `--force` to re-run builds regardless of state.

## Generated Files

Ralph generates shell scripts (aliases, functions, env exports) in:

```
~/.config/ralph/generated/
```

These files are sourced from your shell's RC file via a managed block that ralph injects automatically.

## Complete Example

```toml
dotfiles_repo_path = "~/.dotfiles"
packages_dir = "~/.config/ralph/pkg"

[dotfiles.bashrc]
source = "bash/.bashrc"
target = "~/.bashrc"

[dotfiles.gitconfig]
source = "git/gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true

[dotfiles.nvim]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[directories.local_bin]
target = "~/.local/bin"

[repos.tmux-tpm]
url = "https://github.com/tmux-plugins/tpm"
target = "~/.tmux/plugins/tpm"
update = true

[[tools]]
name = "fzf"
check_command = "command -v fzf"
install_hint = "brew install fzf"

[[tools]]
name = "ripgrep"
check_command = "command -v rg"
install_hint = "brew install ripgrep"

[shell]
name = "zsh"

[shell.aliases.ll]
command = "ls -alh"

[shell.aliases.k]
command = "kubectl"
hosts = ["work-laptop"]

[shell.functions.mkcd]
body = """
mkdir -p "$1" && cd "$1"
"""

[shell.env]
EDITOR = "nvim"
GOPATH = "$HOME/go"

[template_variables]
email = "user@example.com"
font_size = 14

[hooks]
pre_apply = ["echo 'Starting apply...'"]
post_apply = ["echo 'Apply finished.'"]

[hooks.builds.my-tool]
commands = ["go build -o ~/.local/bin/mytool ."]
working_dir = "~/code/mytool"
run = "once"

[packages.neovim]
source = "remote"
repo = "https://github.com/neovim/neovim"
branch = "stable"
build = ["make CMAKE_BUILD_TYPE=Release"]
install = ["sudo make install"]

[recipes_config]
auto_discover = true
dir = "recipes"
exclude = ["experimental/*"]
```
