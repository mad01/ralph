# dotter 🚀

just for fun to play with dotfiles 

**dotter** is a command-line interface (CLI) tool, written in Go, for managing your dotfiles, shell configurations (aliases, functions), and ensuring your favorite shell tools are set up correctly. It's inspired by tools like Starship and aims to provide a declarative way to manage your shell environment through a simple TOML configuration file.

## Philosophy

- **Configuration over Code:** Define your environment declaratively in a `config.toml` file.
- **Idempotency:** Applying your configuration multiple times should result in the same state.
- **Simplicity:** Easy to understand and use for managing common dotfile and shell setups.
- **Extensibility (Future):** While the core is simple, the project considers future extensibility.

## Features (Planned & Implemented)

- **Dotfile Actions**: Manage dotfiles with multiple action types:
  - `symlink` (default): Create symlinks to files
  - `symlink_dir`: Create symlinks to entire directories (like `ln -sfn`)
  - `copy`: Copy files instead of symlinking
- **Directory Management**: Create directories with configurable permissions
- **Repository Management**: Clone and manage git repositories
- **Build Hooks**: Run build commands with configurable run modes (`always`, `once`, `manual`)
- Process dotfiles with Go templating (access to environment variables and config values).
- Define and apply shell aliases and functions for various shells (Bash, Zsh, Fish).
- Inject necessary sourcing lines into your shell's RC file (`.bashrc`, `.zshrc`, `config.fish`).
- Check for the presence of specified tools and provide installation hints.
- `dotter init`: Initialize a new `dotter` configuration.
- `dotter apply`: Apply all defined configurations (with `--force` to re-run builds).
- `dotter list`: List managed items and their status.
- `dotter doctor`: Check the health of your `dotter` setup.
- `--dry-run` global flag: See what changes would be made without executing them.

## Installation

### Using `go install`

If you have Go installed and configured (Go version 1.18+ recommended):

```bash
go install github.com/mad01/dotter@latest
```

This will install the `dotter` binary to your Go bin directory (e.g., `$GOPATH/bin` or `$HOME/go/bin`). Ensure this directory is in your `PATH`.

### From Source

```bash
git clone https://github.com/mad01/dotter.git
cd dotter
go build -o dotter ./cmd/dotter/
# Then move `dotter` to a directory in your PATH, e.g., /usr/local/bin or ~/bin
```

### Binaries from Releases

(Once releases are set up) Pre-compiled binaries for various operating systems will be available on the [GitHub Releases](https://github.com/mad01/dotter/releases) page.

## Configuration (`config.toml`)

`dotter` uses a TOML file for configuration. By default, it looks for this file at:
- `$XDG_CONFIG_HOME/dotter/config.toml`
- `~/.config/dotter/config.toml` (if `$XDG_CONFIG_HOME` is not set)

**It is highly recommended to keep your `config.toml` within your version-controlled dotfiles repository and then symlink it to the default location.**

Run `dotter init` to create a default configuration file.

### Example `config.toml` Structure:

```toml
# Path to your dotfiles source repository (supports ~ expansion)
dotfiles_repo_path = "~/.dotfiles_src"

# === Directories to Create ===
# Create directories before other operations
[directories.dotter_config]
target = "~/.config/dotter"
mode = "0755"                 # Optional, defaults to 0755

[directories.nvim_plugin_dir]
target = "~/.local/share/nvim/site/pack/paqs/opt"

# === Git Repositories ===
# Clone repositories (processed after directories, before dotfiles)
[repos.paq_nvim]
url = "https://github.com/savq/paq-nvim.git"
target = "~/.local/share/nvim/site/pack/paqs/opt/paq-nvim"
# update = false              # Optional: pull latest on each apply
# branch = "main"             # Optional: checkout specific branch
# commit = "abc123"           # Optional: pin to specific commit

# === Dotfiles Management ===
# The key (e.g., "bashrc") is a logical name for the dotfile.

# File symlink (default action)
[dotfiles.bashrc]
source = ".bashrc"            # Relative path within dotfiles_repo_path
target = "~/.bashrc"          # Absolute path on the system (supports ~)
action = "symlink"            # Optional: "symlink" (default), "copy", or "symlink_dir"

# Directory symlink (like ln -sfn)
[dotfiles.nvim_config]
source = "nvim"               # Directory in dotfiles repo
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.kitty_config]
source = "kitty"
target = "~/.config/kitty"
action = "symlink_dir"

# Copy instead of symlink (useful for secrets)
[dotfiles.secrets]
source = "secrets.sh"
target = "~/.secrets.sh"
action = "copy"

# Template processing
[dotfiles.gitconfig_template]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true

# === Build Hooks ===
# Run commands during apply (processed after dotfiles)
[hooks.builds.my_tool]
commands = ["make", "make install"]
working_dir = "~/tools/my-tool"
run = "once"                  # "always", "once", or "manual"

[hooks.builds.another_build]
commands = ["./build.sh"]
working_dir = "~/projects/other"
run = "always"                # Run every time apply is called

# === Tool Management ===
# [[tools]]
# name = "fzf"
# check_command = "command -v fzf"
# install_hint = "Install fzf from https://github.com/junegunn/fzf"

# === Shell Configuration ===
# Aliases use a structured format with command field
[shell.aliases.ll]
command = "ls -alhF"

[shell.aliases.g]
command = "git"

# Aliases can have host filtering
[shell.aliases.work-cmd]
command = "ssh work-server"
hosts = ["work-laptop"]

[shell.functions.my_greeting]
body = '''
  echo "Hello from a dotter-managed function, $1!"
'''
```

### Templating

If `is_template = true` for a dotfile, it will be processed using Go's `text/template` engine before being symlinked. You can use:
- Environment variables: `{{ env "USER" }}`
- Configuration values: `{{ .DotterConfig.DotfilesRepoPath }}` (accesses the main `Config` struct)

#### Go Template Syntax and Features

Dotter leverages Go's powerful templating system, which supports:

**Basic Syntax:**
```
{{ .Variable }}           # Access a variable 
{{ env "ENV_VAR_NAME" }}  # Access environment variable
{{ if .Condition }}       # Conditional logic
  Content if true
{{ else }}
  Content if false
{{ end }}
```

**Accessing Config Variables:**

Template variables defined in your config.toml are directly accessible:
```toml
# In your config.toml
[template_variables]
username = "myuser"
email = "user@example.com"
```

```
# In your template file
Git user: {{ .username }}
Git email: {{ .email }}
```

**Available Variables in Templates:**

- `.DotterConfig`: Access to the full dotter configuration
  - `.DotterConfig.DotfilesRepoPath`: Path to your dotfiles repository
  - `.DotterConfig.TemplateVariables`: Map of template variables
- Environment variables via the `env` function: `{{ env "HOME" }}`
- All keys from `template_variables` section of your config.toml

**Conditional Configuration Example:**

```
# Set different configurations based on OS or hostname
{{ if eq (env "HOSTNAME") "work-laptop" }}
export PROXY="http://work-proxy:8080"
{{ else }}
# No proxy for home computer
{{ end }}

{{ if eq (env "OS") "Darwin" }}
# macOS specific settings
alias ls="ls -G"
{{ else }}
# Linux specific settings
alias ls="ls --color=auto"
{{ end }}
```

**Iteration Example:**

```
# Generate configurations for multiple directories
{{ range $dir := .directories }}
mkdir -p ~/{{ $dir }}
{{ end }}
```

#### Advanced Template Features

Dotter templates support all standard Go template features, including:

- **Functions:** `eq`, `ne`, `lt`, `gt`, `and`, `or`, `not`
- **Pipelines:** `{{ env "HOME" | printf "%s/.local" }}`
- **Comments:** `{{/* This is a comment */}}`
- **Whitespace Control:** `{{- .Variable -}}` trims whitespace before/after

#### Template Example: Dynamic Git Configuration

```
# ~/.dotfiles/.gitconfig.tmpl
[user]
    name = {{ .git_name }}
    email = {{ .git_email }}

[core]
    editor = {{ .editor | default "vim" }}
    
{{ if eq (env "HOSTNAME") "work-laptop" }}
[user]
    # Override email for work machine
    email = {{ .work_email }}
    
[http]
    proxy = {{ .work_proxy }}
{{ end }}
```

With this template, you can define different Git configurations based on your machine, controlled by your `config.toml`.

### Dotfile Actions

Dotter supports three action types for managing dotfiles:

| Action | Description | Use Case |
|--------|-------------|----------|
| `symlink` | Creates a symbolic link to a file (default) | Most dotfiles |
| `symlink_dir` | Creates a symbolic link to a directory | App config directories (nvim, kitty, etc.) |
| `copy` | Copies the file instead of symlinking | Secrets, files that shouldn't be symlinks |

### Directory Management

Create directories before other operations run:

```toml
[directories.my_dir]
target = "~/.config/myapp"
mode = "0755"  # Optional, defaults to 0755
```

Directories are created idempotently - if they already exist, they're skipped.

### Repository Management

Clone and manage git repositories:

```toml
[repos.my_repo]
url = "https://github.com/user/repo.git"
target = "~/path/to/clone"
branch = "main"      # Optional: checkout specific branch
commit = "abc123"    # Optional: pin to specific commit (mutually exclusive with update)
update = true        # Optional: pull latest on each apply
```

**Behavior:**
- If target doesn't exist: clone the repository
- If target exists and `commit` is set: fetch and checkout that commit
- If target exists and `update = true`: pull latest changes
- Otherwise: skip (idempotent)

### Host-based Filtering

Apply configurations only on specific hostnames. This is useful when you share a single config across multiple machines but want certain items to only apply on specific hosts.

```toml
# Apply dotfile only on specific hosts
[dotfiles.work_config]
source = "work.zshrc"
target = "~/.work.zshrc"
hosts = ["work-laptop", "work-desktop"]

# Directory only on certain hosts
[directories.obsidian]
target = "~/workspace/docs"
hosts = ["personal-macbook"]

# Repo only on work machine
[repos.work_tools]
url = "https://github.com/company/tools.git"
target = "~/work/tools"
hosts = ["work-laptop"]

# Tool only on certain hosts
[[tools]]
name = "docker"
check_command = "command -v docker"
install_hint = "Install Docker Desktop"
hosts = ["work-laptop", "server-01"]

# Shell alias with host filtering
[shell.aliases.work-ssh]
command = "ssh work.internal"
hosts = ["work-laptop"]

# Shell function only on specific hosts
[shell.functions.work-setup]
body = '''
echo "Setting up work environment"
'''
hosts = ["work-laptop", "work-desktop"]

# Build only on specific machine
[hooks.builds.work_tools]
commands = ["./install-work-tools.sh"]
working_dir = "~/tools"
run = "once"
hosts = ["work-laptop"]
```

**Behavior:**
- Empty or omitted `hosts` field means the item applies to all hosts (default)
- Hostname matching is case-insensitive
- Items that don't match the current hostname are skipped with a message

### Disabling Config Items

Any config item can be disabled by setting `enable = false`. This is useful for temporarily disabling items without removing them from the config.

```toml
# Disabled dotfile (won't be applied)
[dotfiles.old_config]
source = "old.conf"
target = "~/.old.conf"
enable = false

# Disabled directory
[directories.temp_dir]
target = "~/temp"
enable = false

# Disabled repo
[repos.archived_project]
url = "https://github.com/user/archived.git"
target = "~/archived"
enable = false

# Disabled tool check
[[tools]]
name = "old-tool"
check_command = "command -v old-tool"
install_hint = "deprecated"
enable = false

# Disabled alias
[shell.aliases.old-alias]
command = "echo deprecated"
enable = false

# Disabled function
[shell.functions.unused-func]
body = "echo unused"
enable = false

# Disabled build
[hooks.builds.slow_build]
commands = ["./slow-build.sh"]
run = "once"
enable = false
```

**Behavior:**
- Default (not specified): enabled
- `enable = true`: explicitly enabled
- `enable = false`: disabled, item is skipped

### Build Hooks

Run build commands during apply:

```toml
[hooks.builds.my_build]
commands = ["./configure", "make", "make install"]
working_dir = "~/path/to/project"
run = "once"  # "always", "once", or "manual"
```

**Run modes:**
- `always`: Run on every `dotter apply`
- `once`: Run only if not previously completed (tracked in `~/.config/dotter/.builds_state`)
- `manual`: Only run when explicitly requested with `--build=name`

**Automatic change detection:**
For `once` builds with a `working_dir` that is a git repository, dotter automatically:
- Tracks the git commit hash when the build completes
- Re-runs the build if the commit hash changes
- Re-runs the build if there are uncommitted changes

**Re-triggering builds:**
- Builds with git changes are automatically re-run
- Use `--force` to re-run all `once` builds regardless of state
- Use `--build=name` to run a specific build (including `manual` builds)
- Use `--reset-builds` to clear all build state and start fresh

## Usage

- `dotter init`: Guides you through creating an initial `config.toml`.
- `dotter apply`: Reads your `config.toml` and applies all configurations:
    - Creates configured directories
    - Clones/updates configured repositories
    - Symlinks/copies dotfiles (processing templates if specified)
    - Runs build hooks
    - Generates shell alias and function scripts
    - Injects sourcing lines into your shell's rc file
- `dotter list`: Shows the status of managed dotfiles, configured tools, aliases, and functions.
- `dotter doctor`: Checks your setup for common issues (config validity, broken symlinks, directories, repos, builds, rc file sourcing).
- `dotter --help`: Shows help for all commands and flags.

### Apply Command Flags

- `--overwrite`: Overwrite existing files at target locations
- `--skip`: Skip if target file already exists
- `--force`: Force re-run of `once` builds even if previously completed
- `--build=name`: Run a specific build (works with `manual` builds too)
- `--reset-builds`: Clear all build state and re-run `once` builds

### Global Flags

- `-n`, `--dry-run`: Show what changes would be made without actually executing them.

## Best Practices

1.  **Version Control Your Dotfiles:** Keep your actual dotfiles (the source files) in a Git repository (e.g., `~/.dotfiles_src`).
2.  **Version Control `config.toml`:** Place your `dotter` `config.toml` file inside this same Git repository.
3.  **Symlink `config.toml`:** After placing `config.toml` in your dotfiles repository, symlink it to the expected location (`$XDG_CONFIG_HOME/dotter/config.toml` or `~/.config/dotter/config.toml`).
    Example: `ln -s ~/.dotfiles_src/config.toml ~/.config/dotter/config.toml`
4.  Run `dotter apply` whenever you make changes to your configuration or your dotfiles repository.

## Using `pkg/pipeutil` for Custom Shell Binaries

`dotter` includes a utility package `github.com/mad01/dotter/pkg/pipeutil` to help you write simple Go programs that can act as shell filters or transformers, easily interacting with stdin, stdout, and stderr.

### Features:

- `pipeutil.ReadAll()`: Reads all of `os.Stdin`.
- `pipeutil.Scanner()`: Returns a `bufio.Scanner` for line-by-line reading of `os.Stdin`.
- `pipeutil.Print([]byte)`: Writes to `os.Stdout`.
- `pipeutil.Println(string)`: Writes a string (with a newline) to `os.Stdout`.
- `pipeutil.Error(error)` / `pipeutil.Errorf(format, ...)`: Writes formatted errors to `os.Stderr`.
- `pipeutil.ExitSuccess` / `pipeutil.ExitFailure`: Constants for exit codes.

### Example (`pkg/pipeutil/example/main.go`):

```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mad01/dotter/pkg/pipeutil"
)

func main() {
	binput, err := pipeutil.ReadAll()
	if err != nil {
		pipeutil.Errorf("failed to read from stdin: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	input := string(binput)
	upper := strings.ToUpper(input)

	_, err = pipeutil.Println(upper)
	if err != nil {
		pipeutil.Errorf("failed to write to stdout: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}
	os.Exit(pipeutil.ExitSuccess)
}
```

**To compile and use such a binary:**

```bash
# Assuming you are in the directory of your custom tool (e.g., pkg/pipeutil/example)
go build -o mytool
echo "some input" | ./mytool
```

## Recipes: Modular Configuration

Recipes allow you to split your configuration into modular `recipe.toml` files that live alongside your source files. This makes it easy to:
- Organize related configs together (e.g., all Kubernetes stuff in one place)
- Enable/disable entire feature sets per machine
- Share and reuse configuration modules

### Recipe File Format

A recipe file is a `recipe.toml` placed in a directory within your dotfiles repo:

```toml
# ~/.dotfiles/editors/recipe.toml
[recipe]
name = "editors"
description = "Editor configurations (nvim, vim)"

# Paths are relative to the recipe directory
[dotfiles.nvim_config]
source = "nvim"                    # Resolves to editors/nvim
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.ideavimrc]
source = "ideavimrc"               # Resolves to editors/ideavimrc
target = "~/.ideavimrc"

[shell.aliases.vim]
command = "nvim"
```

### Using Recipes

**Mode A: Explicit Recipe List (Recommended)**

```toml
# ~/.config/dotter/config.toml
dotfiles_repo_path = "~/.dotfiles"

[[recipes]]
path = "editors/recipe.toml"

[[recipes]]
path = "terminals/recipe.toml"

[[recipes]]
path = "kubernetes/recipe.toml"
hosts = ["work-laptop"]          # Only load on work machine

[[recipes]]
path = "experimental/recipe.toml"
enable = false                    # Disabled
```

**Mode B: Auto-Discovery**

```toml
dotfiles_repo_path = "~/.dotfiles"

[recipes_config]
auto_discover = true              # Find all recipe.toml files
exclude = ["experimental/*"]      # Exclude patterns

[recipes_config.overrides.kubernetes]
hosts = ["work-laptop"]           # Override for specific recipe
```

### Recipe Features

- **Path Resolution**: Relative paths in recipes are resolved relative to the recipe directory
- **Host Filtering**: Recipe-level `hosts` filter applies to all items that don't have their own
- **Conflict Detection**: Errors if the same item name appears in multiple recipes
- **Backward Compatible**: Configs without recipes work unchanged

### Migration Support

When reorganizing your dotfiles repo, existing symlinks will break. Recipes support `legacy_paths` to handle this:

```toml
# editors/recipe.toml
[recipe]
name = "editors"

# Map old paths to new paths for migration
[recipe.legacy_paths]
"dotter_files/nvim" = "nvim"
"dotter_files/ideavimrc" = "ideavimrc"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"
```

Then run:

```bash
# Preview changes
dotter migrate --dry-run

# Update symlinks
dotter migrate

# Verify everything works
dotter apply
```

## Contributing

(CONTRIBUTING.md to be created if contributions are sought)

## License

(LICENSE file to be added - e.g., MIT, Apache 2.0)
