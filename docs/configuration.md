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
| `hosts` | string array | `[]` (all hosts) | Apply only on the listed hostnames. Matching is case-insensitive on the short name: `myhost` matches `myhost.local` and `myhost.example.com`. Empty means all hosts. |

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

### `[dirs_mirror.<name>]`

Walks a source directory and symlinks each entry into a target directory. Entries with a `.` prefix (hidden files) are skipped.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `source` | string | yes | -- | Source directory path, relative to `dotfiles_repo_path`. |
| `target` | string | yes | -- | Target directory path. Supports `~`. |
| `action` | string | no | `"symlink"` | `"symlink"` to symlink each file, or `"symlink_dir"` to symlink each subdirectory. |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

```toml
[dirs_mirror.claude_skills]
source = "skills"
target = "~/.claude/skills"
action = "symlink_dir"

[dirs_mirror.zsh_rc]
source = "rc"
target = "~/.config/zsh/rc"
```

### `[repos.<name>]`

Defines a git repository to clone and optionally keep updated.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | Git repository URL. |
| `target` | string | yes | -- | Local clone path. Supports `~`. |
| `branch` | string | no | -- | Branch to checkout after cloning. |
| `commit` | string | no | -- | Pin to a specific commit hash. |
| `update` | bool | no | `false` | Pull latest changes on each `ralph up`. |
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

Simple key-value pairs for environment variables. Ralph exports these in `~/.config/ralph/generated/generated_env.sh`, which is sourced from your shell RC file before aliases and functions. Host filtering is not supported on individual env vars.

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

Lifecycle hooks that run shell commands at specific points during `ralph up`.

| Field | Type | Description |
|-------|------|-------------|
| `pre_apply` | string array | Commands to run before any apply operations. |
| `post_apply` | string array | Commands to run after all apply operations. |
| `pre_uninstall` | string array | Commands to run during cleanup, before a removed/disabled recipe's artifacts are removed. |
| `post_uninstall` | string array | Commands to run during cleanup, after a removed/disabled recipe's artifacts are removed. |

```toml
[hooks]
pre_apply = ["echo 'Starting apply...'"]
post_apply = ["echo 'Apply finished.'"]
```

#### `[hooks.pre_uninstall]` and `[hooks.post_uninstall]`

Recipe-level cleanup hooks. They run when a recipe becomes an orphan — i.e. it
was disabled (`ralph disable <recipe>`) or removed, and cleanup runs via
`ralph up --enable-cleanup` or `ralph clean`. Use them to tear down external
state that ralph doesn't track as an artifact: unregistering MCP servers,
removing a service from a process manager, uninstalling git hooks.

```toml
[hooks]
pre_uninstall = ["t-man remove myservice 2>/dev/null || true"]
post_uninstall = ["echo 'myservice cleaned up'"]
```

`pre_uninstall` runs before artifact removal and `post_uninstall` after, for
both `delete` and `abandon` recipes (the hooks clean external state, not files).
The commands are **persisted into the recipe manifest** (`~/.config/ralph/.recipe_state`)
at apply time, so they still run even if the recipe's `recipe.toml` is gone from
disk by the time cleanup happens. A hook that fails is logged as a warning and
does not abort cleanup or change the exit code. Under `--dry-run` the commands
are previewed, not executed.

#### `[hooks.pre_link]` and `[hooks.post_link]`

Maps of dotfile name to a list of commands. These run before/after linking a specific dotfile.

```toml
[hooks.pre_link]
nvim_config = ["echo 'About to link nvim config'"]

[hooks.post_link]
bashrc = ["source ~/.bashrc"]
```

#### `[hooks.builds.<name>]`

Build hooks run during `ralph up` after dotfiles and shell configuration are processed.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `commands` | string array | no* | -- | Shell commands to execute in order. Required unless `script` is set. |
| `script` | string | no | -- | Path to a shell script to execute, relative to `working_dir`. Alternative to `commands` — provide one or the other, not both. |
| `working_dir` | string | no | -- | Directory to run commands in. Supports `~`. |
| `run` | string | yes | -- | Run mode: `"always"`, `"once"`, or `"manual"`. |
| `idempotent` | bool | no | `false` | Skip the build when its content hash matches the last successful run. See [Idempotent builds](#idempotent-builds). |
| `timeout` | int | no | `600` | Maximum execution time in seconds for the build commands. Set to 0 or omit for the 600-second default. |
| `depends_on` | string array | no | `[]` | Items that must complete before this build runs. Format: `"builds.<name>"` or `"packages.<name>"`. See [Dependency ordering](#dependency-ordering). |
| `install_paths` | string array | no | `[]` | Declarative list of files this build writes to disk. Used by cleanup to remove orphaned binaries when the recipe goes away. See [Install paths](#install-paths). |
| `hosts` | string array | no | `[]` | Host filtering. |
| `enable` | bool (pointer) | no | `nil` | Enable/disable. |

Run modes:

- `"always"` -- runs on every `ralph up`.
- `"once"` -- runs once and is skipped on subsequent runs (unless `--force` or `--reset-builds` is used).
- `"manual"` -- only runs when explicitly requested via `--build=NAME`.

```toml
[hooks.builds.my-tool]
commands = ["go build -o ~/.local/bin/mytool ."]
working_dir = "~/code/mytool"
run = "once"
install_paths = ["~/.local/bin/mytool"]

[hooks.builds.my_tool_script]
script = "build.sh"
working_dir = "~/code/my-tool"
run = "once"

[hooks.builds.post_process]
commands = ["./post-process.sh"]
working_dir = "~/code/mytool"
run = "always"
timeout = 120
depends_on = ["builds.my-tool"]
```

#### Idempotent builds

`idempotent = true` adds a fast pre-check: ralph computes `sha256(name + commands + working_dir)` and compares it to the hash stored in `~/.config/ralph/.builds_state`. A match prints `Build 'X' content unchanged. Skipping (idempotent).` and exits early without running the commands. A mismatch (or no prior record) runs the build and persists the new hash on success.

Combine with any `run` mode. With `run = "always"`, idempotent means "rerun only when the recipe edits the command or the cwd". With `run = "once"`, the content hash supplements the existing git-hash and uncommitted-changes checks.

`--force` bypasses the idempotent skip.

The hash is over the *command string*, not files the command reads. Do not enable `idempotent` on commands that read mutable inputs (sync scripts that diff a JSON config, hook installers that walk a repo set) — the apply will skip after the first run even when the underlying inputs have changed. For those, leave `idempotent = false` and let them run every time.

### `[packages.<name>]`

Managed packages synced and built during `ralph up`. Packages track build state separately from build hooks, using `pkg:` prefixed keys in the state file.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `source` | string | yes | -- | `"local"`, `"remote"`, `"make"`, or `"go-install"`. |
| `repo` | string | remote/make | -- | Git URL for remote and make packages. |
| `target` | string | no | `<packages_dir>/<name>` | Clone target directory for remote/make packages. |
| `branch` | string | no | -- | Branch to track (remote/make only). |
| `module` | string | go-install only | -- | Go module path for `go install` (e.g. `"github.com/user/tool/cmd/tool"`). |
| `version` | string | go-install only | -- | Version tag for `go install` (e.g. `"v1.2.3"`). |
| `working_dir` | string | no | `target` (remote) | Directory to run build/install commands in. Supports `~`. |
| `build` | string array | conditional | -- | Build commands. Required for `local` and `remote`. Defaults to `["make build"]` for `make`. Not used by `go-install`. |
| `install` | string array | no | `[]` | Install commands. Defaults to `["make install"]` for `make`. Not used by `go-install`. |
| `timeout` | int | no | `600` | Maximum execution time in seconds for build/install commands. Set to 0 or omit for the 600-second default. |
| `depends_on` | string array | no | `[]` | Items that must complete before this package builds. Format: `"builds.<name>"` or `"packages.<name>"`. See [Dependency ordering](#dependency-ordering). |
| `install_paths` | string array | no | `[]` | Declarative list of files this package writes to disk (e.g. `["~/code/bin/foo"]`). Used by cleanup. See [Install paths](#install-paths). For `go-install`, GOBIN is set to the directory of the first entry. |
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
install_paths = ["~/.local/bin/my-cli"]

[packages.my-tool]
source = "make"
repo = "git@github.com:user/tool.git"
install_paths = ["~/code/bin/tool"]

[packages.github_mcp_server]
source = "go-install"
module = "github.com/github/github-mcp-server/cmd/github-mcp-server"
version = "v1.0.5"
install_paths = ["~/code/bin/github-mcp-server"]
```

#### Install paths

`install_paths` is a hand-written list of every file the build or install commands write to disk. Ralph cannot inspect what `make install` does, so this list is the source of truth for cleanup.

Rules enforced when ralph removes an entry:

- No glob characters (`*`, `?`, `[`, `]`, `{`, `}`)
- Path must be inside `$HOME`
- Path must resolve to a regular file or symlink (not a directory)
- A missing file is logged and skipped, not an error
- A path still declared by any active package or build in the current manifest is never removed, even if the recipe that previously owned it is now an orphan
- The currently-running `ralph` binary (or a symlink to it) is never removed

If you do not declare `install_paths`, the package is still tracked but cleanup logs it as `abandoned package: NAME (declare install_paths to enable cleanup)` and leaves the binary in place.

#### Dependency ordering

Builds and packages support a `depends_on` field that declares execution dependencies. During `ralph up`, builds and packages run in a single unified phase, ordered by topological sort (Kahn's algorithm).

Each entry in `depends_on` uses the format `"builds.<name>"` or `"packages.<name>"`:

```toml
[hooks.builds.compile_index]
commands = ["brain index"]
run = "always"
depends_on = ["packages.brain", "builds.pull_models"]

[packages.brain]
source = "make"
repo = "git@github.com:user/brain.git"
install_paths = ["~/code/bin/brain"]
```

Rules:

- Dependency references must exist in the config. Dangling references cause a validation error.
- Circular dependencies are detected and rejected at config validation time.
- Items with no dependencies maintain alphabetical order relative to each other.
- Cross-type dependencies are allowed: a build can depend on a package and vice versa.

Above the topological sort sits a coarser layer: waves. Builds and packages are grouped by their recipe's `wave` value (recipe default `1`, main-config items `0`) and lower waves complete before higithostr waves start. `depends_on` orders items within a wave; a reference to an item in a later wave is a validation error. See [Build ordering with waves](recipes.md#build-ordering-with-waves).

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
| `auto_cleanup` | bool | no | `false` | Run cleanup on every `ralph up` without needing `--enable-cleanup`. |
| `dir` | string | no | `"recipes"` | Directory to search, relative to `dotfiles_repo_path`. |
| `exclude` | string array | no | `[]` | Glob patterns to exclude from auto-discovery. |

```toml
[recipes_config]
auto_discover = true
auto_cleanup = true
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
wave = 1  # default; lower waves build first (see recipes.md)
delete_behavior = "delete"  # default; "abandon" leaves orphans in place

[recipe.legacy_paths]
"ralph_files/nvim/init.lua" = "nvim/init.lua"

[dotfiles.nvim_init]
source = "init.lua"
target = "~/.config/nvim/init.lua"

[shell.aliases.vim]
command = "nvim"
```

The `delete_behavior` field controls cleanup when the recipe is removed from your config or disabled. See [recipes](recipes.md#recipe-deletion-and-cleanup) for the full deletion model.

## State files

Ralph keeps two JSON state files under `~/.config/ralph/`. State files are written atomically (write to temp file, then rename), so a crash during apply never leaves corrupted state.

### `.builds_state`

Tracks build and package completion.

- Build hooks use their name as the key (e.g., `"my-tool"`).
- Packages use a `pkg:` prefix (e.g., `"pkg:neovim"`).
- Each entry records the completion timestamp, the subtree tree hash of `working_dir` at build time (when available — see below), and the content hash for [idempotent builds](#idempotent-builds).

For `run = "once"` builds and remote/make/local packages, freshness is keyed on the **tree hash** of the `working_dir` subtree (`git rev-parse HEAD:<subdir>`) rather than the repository's HEAD commit. A rebuild is triggered only when that subtree's contents change or it has uncommitted (non-ignored) modifications — commits elsewhere in the repo leave the build cached. State written by older ralph versions holds a commit hash, so the first run after upgrading rebuilds once and then records the tree hash.

Use `--reset-builds` on `ralph up` to clear all build state, or `--force` to re-run builds regardless of state. Neither is required to restore a deleted binary: when a package's declared `install_path` is missing from disk, a plain `ralph up` rebuilds it even if the source is unchanged.

### `.recipe_state`

Per-recipe artifact manifest. Written at the end of every `ralph up --enable-cleanup` and read at the start of the next one to compute orphans.

Each entry records the recipe's `delete_behavior` and the artifacts it owns: symlinks, copies, directories, repos, shell aliases/functions/env vars, package and build names, and `install_paths`.

Inspect the manifest with `ralph state show` (or `ralph state show --json` for the raw form). See [recipes](recipes.md#recipe-deletion-and-cleanup) for the cleanup workflow.

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

[dirs_mirror.zsh_rc]
source = "rc"
target = "~/.config/zsh/rc"

[hooks.builds.my-tool]
commands = ["go build -o ~/.local/bin/mytool ."]
working_dir = "~/code/mytool"
run = "once"
timeout = 300

[packages.neovim]
source = "remote"
repo = "https://github.com/neovim/neovim"
branch = "stable"
build = ["make CMAKE_BUILD_TYPE=Release"]
install = ["sudo make install"]

[packages.my-cli]
source = "make"
repo = "git@github.com:user/my-cli.git"
install_paths = ["~/code/bin/my-cli"]
depends_on = ["packages.neovim"]

[packages.gh_mcp]
source = "go-install"
module = "github.com/github/github-mcp-server/cmd/github-mcp-server"
version = "v1.0.5"
install_paths = ["~/code/bin/github-mcp-server"]

[recipes_config]
auto_discover = true
auto_cleanup = true
dir = "recipes"
exclude = ["experimental/*"]
```
