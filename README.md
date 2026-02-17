# ralph

*"Me fail dotfiles? That's unpossible."*

ralph is a dotfiles manager written in Go. You tell it what goes where in a TOML file, and it puts things there. Symlinks, copies, shell aliases, git repos, build hooks -- ralph handles the boring parts so you can stop hand-wiring your `.bashrc` like it's 2003.

Named after Ralph Wiggum, who once dragged his entire `~/.config` into the trash and told Miss Hoover his computer had "the hiccups." This tool exists so you don't have to be Ralph.

## Philosophy

**Configuration over code.** Your setup lives in a `config.toml` file, not scattered across shell scripts you wrote at 2am and no longer understand. Declare what you want. ralph figures out the rest.

**Idempotent, like Ralph's lunch order.** Run `ralph apply` once or fifty times -- you get the same result. No surprises, no duplicated symlinks, no "why is my shell broken now" moments.

**Simple enough for Ralph.** Well, almost. There's no plugin system, no daemon, no twelve-layer abstraction. It reads your config, it does the thing, it stops. If you need more than that, you might be overthinking your dotfiles.

## Quick start

*"I'm learnding!"*

### 1. Install ralph

Pick your poison:

```bash
# If you have Go installed (1.21+)
go install github.com/mad01/ralph/cmd/ralph@latest

# Or build from source
git clone https://github.com/mad01/ralph.git
cd ralph
make build
# Move the binary somewhere in your PATH
mv ralph /usr/local/bin/
```

Pre-built binaries are also available on the [Releases](https://github.com/mad01/ralph/releases) page.

### 2. Initialize your config

```bash
ralph init
```

This walks you through creating a config file at `~/.config/ralph/config.toml`. It'll ask where your dotfiles repo lives (default: `~/.dotfiles`).

### 3. Set up your dotfiles repo

If you don't have a dotfiles repo yet, create one:

```bash
mkdir ~/.dotfiles
# Move your configs there
cp ~/.bashrc ~/.dotfiles/.bashrc
cp ~/.gitconfig ~/.dotfiles/.gitconfig
```

### 4. Tell ralph what to manage

Open `~/.config/ralph/config.toml` and add your dotfiles:

```toml
dotfiles_repo_path = "~/.dotfiles"

[dotfiles.bashrc]
source = ".bashrc"
target = "~/.bashrc"

[dotfiles.gitconfig]
source = ".gitconfig"
target = "~/.gitconfig"

[shell.aliases.ll]
command = "ls -alhF"

[shell.aliases.g]
command = "git"
```

### 5. Apply it

```bash
ralph apply
```

Done. Want to see what would happen without actually doing anything? Use dry run:

```bash
ralph apply --dry-run
```

### What just happened?

When you ran `ralph apply`, it went through your config and:

- Symlinked your dotfiles from `~/.dotfiles/.bashrc` to `~/.bashrc` (and so on for each entry)
- Generated shell config files for your aliases and functions
- Injected source lines into your shell's rc file so aliases and functions load in every new terminal
- Checked any tools you listed and told you which ones are missing
- Cloned git repos if you configured any
- Ran build hooks if you set those up

Run it again and nothing changes. Run it after updating your config and only the diff gets applied.

### Useful flags



```
ralph apply --overwrite    # Overwrite existing files at target locations
ralph apply --skip         # Skip if target already exists
ralph apply --force        # Re-run one-time builds
ralph apply --dry-run      # Preview changes without doing anything
ralph doctor               # Check your setup for problems
ralph list                 # See what ralph is managing
```

## Configuration (`config.toml`)

ralph uses a TOML file for configuration. By default, it looks for this file at:
- `$XDG_CONFIG_HOME/ralph/config.toml`
- `~/.config/ralph/config.toml` (if `$XDG_CONFIG_HOME` is not set)

Keep your `config.toml` in your dotfiles repo and symlink it to the config location.

Run `ralph init` to create a default configuration file.

### Config structure

```toml
# Path to your dotfiles source repository (supports ~ expansion)
dotfiles_repo_path = "~/.dotfiles_src"

# === Directories to Create ===
[directories.ralph_config]
target = "~/.config/ralph"
mode = "0755"                 # Optional, defaults to 0755

[directories.nvim_plugin_dir]
target = "~/.local/share/nvim/site/pack/paqs/opt"

# === Git Repositories ===
[repos.paq_nvim]
url = "https://github.com/savq/paq-nvim.git"
target = "~/.local/share/nvim/site/pack/paqs/opt/paq-nvim"
# update = false              # Optional: pull latest on each apply
# branch = "main"             # Optional: checkout specific branch
# commit = "abc123"           # Optional: pin to specific commit

# === Dotfiles ===
[dotfiles.bashrc]
source = ".bashrc"            # Relative path within dotfiles_repo_path
target = "~/.bashrc"          # Absolute path on the system (supports ~)
action = "symlink"            # Optional: "symlink" (default), "copy", or "symlink_dir"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"        # Directory symlink (like ln -sfn)

[dotfiles.secrets]
source = "secrets.sh"
target = "~/.secrets.sh"
action = "copy"               # Copy instead of symlink

[dotfiles.gitconfig_template]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true            # Process with Go templates

# === Build Hooks ===
[hooks.builds.my_tool]
commands = ["make", "make install"]
working_dir = "~/tools/my-tool"
run = "once"                  # "always", "once", or "manual"

# === Tools ===
[[tools]]
name = "fzf"
check_command = "command -v fzf"
install_hint = "https://github.com/junegunn/fzf"

# === Shell ===
[shell.aliases.ll]
command = "ls -alhF"

[shell.aliases.g]
command = "git"

[shell.aliases.work-cmd]
command = "ssh work-server"
hosts = ["work-laptop"]       # Host filtering

[shell.functions.my_greeting]
body = '''
  echo "Hello from a ralph-managed function, $1!"
'''
```

### Dotfile actions

| Action | Description | Use Case |
|--------|-------------|----------|
| `symlink` | Creates a symbolic link to a file (default) | Most dotfiles |
| `symlink_dir` | Creates a symbolic link to a directory | App config directories (nvim, kitty, etc.) |
| `copy` | Copies the file instead of symlinking | Secrets, files that shouldn't be symlinks |

### Directory management

Create directories before other operations run:

```toml
[directories.my_dir]
target = "~/.config/myapp"
mode = "0755"  # Optional, defaults to 0755
```

Directories are created idempotently -- if they already exist, they're skipped.

### Repository management

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

### Host-based filtering

Apply configurations only on specific hostnames.

```toml
[dotfiles.work_config]
source = "work.zshrc"
target = "~/.work.zshrc"
hosts = ["work-laptop", "work-desktop"]

[directories.obsidian]
target = "~/workspace/docs"
hosts = ["personal-macbook"]

[repos.work_tools]
url = "https://github.com/company/tools.git"
target = "~/work/tools"
hosts = ["work-laptop"]

[[tools]]
name = "docker"
check_command = "command -v docker"
install_hint = "Install Docker Desktop"
hosts = ["work-laptop", "server-01"]

[shell.aliases.work-ssh]
command = "ssh work.internal"
hosts = ["work-laptop"]

[hooks.builds.work_tools]
commands = ["./install-work-tools.sh"]
working_dir = "~/tools"
run = "once"
hosts = ["work-laptop"]
```

**Behavior:**
- Empty or omitted `hosts` field means the item applies to all hosts (default)
- Hostname matching is case-insensitive
- Items that don't match the current hostname are skipped

### Disabling config items

Any config item can be disabled with `enable = false`. Handy for temporarily turning things off without removing them.

```toml
[dotfiles.old_config]
source = "old.conf"
target = "~/.old.conf"
enable = false

[hooks.builds.slow_build]
commands = ["./slow-build.sh"]
run = "once"
enable = false
```

**Behavior:**
- Default (not specified): enabled
- `enable = true`: explicitly enabled
- `enable = false`: disabled, item is skipped

### Build hooks

Run build commands during apply:

```toml
[hooks.builds.my_build]
commands = ["./configure", "make", "make install"]
working_dir = "~/path/to/project"
run = "once"  # "always", "once", or "manual"
```

**Run modes:**
- `always`: Run on every `ralph apply`
- `once`: Run only if not previously completed (tracked in `~/.config/ralph/.builds_state`)
- `manual`: Only run when explicitly requested with `--build=name`

**Automatic change detection:**
For `once` builds with a `working_dir` that is a git repository, ralph automatically:
- Tracks the git commit hash when the build completes
- Re-runs the build if the commit hash changes
- Re-runs the build if there are uncommitted changes

**Re-triggering builds:**
- Builds with git changes are automatically re-run
- Use `--force` to re-run all `once` builds regardless of state
- Use `--build=name` to run a specific build (including `manual` builds)
- Use `--reset-builds` to clear all build state and start fresh

### Templating

If `is_template = true` for a dotfile, it gets processed with Go's `text/template` engine before being symlinked.

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

**Config Variables:**

Define variables in your config, use them in templates:
```toml
# In config.toml
[template_variables]
username = "myuser"
email = "user@example.com"
```

```
# In your template file
Git user: {{ .username }}
Git email: {{ .email }}
```

**Available in templates:**
- `.RalphConfig`: Full ralph configuration object
  - `.RalphConfig.DotfilesRepoPath`: Path to your dotfiles repository
  - `.RalphConfig.TemplateVariables`: Map of template variables
- `env` function: `{{ env "HOME" }}`
- All keys from `template_variables`

**Conditional example:**
```
{{ if eq (env "HOSTNAME") "work-laptop" }}
export PROXY="http://work-proxy:8080"
{{ end }}

{{ if eq (env "OS") "Darwin" }}
alias ls="ls -G"
{{ else }}
alias ls="ls --color=auto"
{{ end }}
```

**Git config template example:**
```
# ~/.dotfiles/.gitconfig.tmpl
[user]
    name = {{ .git_name }}
    email = {{ .git_email }}

[core]
    editor = {{ .editor | default "vim" }}

{{ if eq (env "HOSTNAME") "work-laptop" }}
[user]
    email = {{ .work_email }}

[http]
    proxy = {{ .work_proxy }}
{{ end }}
```

Go template features: `eq`, `ne`, `lt`, `gt`, `and`, `or`, `not`, pipelines (`{{ env "HOME" | printf "%s/.local" }}`), comments (`{{/* comment */}}`), whitespace control (`{{- .Variable -}}`).

## Recipes

Recipes let you split your configuration into `recipe.toml` files that live alongside your source files.

### Recipe file format

A `recipe.toml` in a directory within your dotfiles repo:

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

### Using recipes

**Explicit recipe list (recommended):**

```toml
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

**Auto-discovery:**

```toml
dotfiles_repo_path = "~/.dotfiles"

[recipes_config]
auto_discover = true              # Find all recipe.toml files
exclude = ["experimental/*"]      # Exclude patterns

[recipes_config.overrides.kubernetes]
hosts = ["work-laptop"]           # Override for specific recipe
```

### Recipe features

- Relative paths in recipes resolve relative to the recipe directory
- Recipe-level `hosts` filter applies to all items that don't set their own
- Errors if the same item name appears in multiple recipes
- Configs without recipes work unchanged

### Migration support

When reorganizing your dotfiles repo, existing symlinks break. Recipes support `legacy_paths` to handle this:

```toml
# editors/recipe.toml
[recipe]
name = "editors"

[recipe.legacy_paths]
"ralph_files/nvim" = "nvim"
"ralph_files/ideavimrc" = "ideavimrc"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"
```

```bash
ralph migrate --dry-run    # Preview changes
ralph migrate              # Update symlinks
ralph apply                # Verify everything works
```

## Real-world examples

Practical configs you can steal and adapt.

### The developer workstation

A single-machine setup for someone who lives in the terminal. Git shortcuts, container aliases, editor configs, and a few functions that save more time than they took to write.

```toml
dotfiles_repo_path = "~/.dotfiles"

[template_variables]
git_name = "Your Name"
git_email = "you@example.com"
editor = "nvim"

# --- Directories ---

[directories.nvim_data]
target = "~/.local/share/nvim/site/pack/plugins/opt"

[directories.local_bin]
target = "~/.local/bin"

# --- Dotfiles ---

[dotfiles.gitconfig]
source = "git/.gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true

[dotfiles.gitignore_global]
source = "git/.gitignore_global"
target = "~/.gitignore_global"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.tmux_conf]
source = "tmux/.tmux.conf"
target = "~/.tmux.conf"

[dotfiles.starship]
source = "starship/starship.toml"
target = "~/.config/starship.toml"

# --- Shell ---

# Git shortcuts -- because typing "git" every time is a tax on your wrists
[shell.aliases.g]
command = "git"
[shell.aliases.gs]
command = "git status -sb"
[shell.aliases.gc]
command = "git commit"
[shell.aliases.gca]
command = "git commit --amend"
[shell.aliases.gd]
command = "git diff"
[shell.aliases.gds]
command = "git diff --staged"
[shell.aliases.gl]
command = "git log --oneline --graph --decorate -20"
[shell.aliases.gp]
command = "git push"
[shell.aliases.gpf]
command = "git push --force-with-lease"

# Container / orchestration
[shell.aliases.dk]
command = "docker"
[shell.aliases.dc]
command = "docker compose"
[shell.aliases.k]
command = "kubectl"
[shell.aliases.kgp]
command = "kubectl get pods"
[shell.aliases.kgs]
command = "kubectl get svc"
[shell.aliases.kgn]
command = "kubectl get nodes"
[shell.aliases.kctx]
command = "kubectx"
[shell.aliases.kns]
command = "kubens"

# Day-to-day
[shell.aliases.vim]
command = "nvim"
[shell.aliases.cat]
command = "bat --paging=never"
[shell.aliases.ls]
command = "eza --icons"
[shell.aliases.ll]
command = "eza -l --icons --git"
[shell.aliases.la]
command = "eza -la --icons --git"
[shell.aliases.tree]
command = "eza --tree --icons --level=3"

# Functions
[shell.functions.mkcd]
body = '''
  mkdir -p "$1" && cd "$1"
'''

[shell.functions.port]
body = '''
  lsof -i ":$1"
'''

[shell.functions.extract]
body = '''
  case "$1" in
    *.tar.bz2) tar xjf "$1" ;;
    *.tar.gz)  tar xzf "$1" ;;
    *.tar.xz)  tar xJf "$1" ;;
    *.bz2)     bunzip2 "$1" ;;
    *.gz)      gunzip "$1" ;;
    *.tar)     tar xf "$1" ;;
    *.zip)     unzip "$1" ;;
    *.7z)      7z x "$1" ;;
    *)         echo "don't know how to extract '$1'" ;;
  esac
'''

# --- Tools ---
[[tools]]
name = "nvim"
check_command = "command -v nvim"
install_hint = "https://neovim.io/"

[[tools]]
name = "starship"
check_command = "command -v starship"
install_hint = "https://starship.rs/"

[[tools]]
name = "eza"
check_command = "command -v eza"
install_hint = "cargo install eza"

[[tools]]
name = "bat"
check_command = "command -v bat"
install_hint = "https://github.com/sharkdop/bat"

[[tools]]
name = "fzf"
check_command = "command -v fzf"
install_hint = "https://github.com/junegunn/fzf"
```

### Multi-machine setup

Ralph eats paste, but he knows which computer he's on. Use `hosts` to keep work stuff off your personal laptop and vice versa.

```toml
dotfiles_repo_path = "~/.dotfiles"

[template_variables]
personal_email = "you@gmail.com"
work_email = "you@company.com"

# --- Shared across all machines ---

[dotfiles.zshrc]
source = "zsh/.zshrc"
target = "~/.zshrc"

[dotfiles.gitignore_global]
source = "git/.gitignore_global"
target = "~/.gitignore_global"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[shell.aliases.g]
command = "git"
[shell.aliases.ll]
command = "ls -alhF"
[shell.aliases.vim]
command = "nvim"

[shell.functions.mkcd]
body = '''
  mkdir -p "$1" && cd "$1"
'''

# --- Personal machine only ---

[dotfiles.gitconfig_personal]
source = "git/.gitconfig-personal.tmpl"
target = "~/.gitconfig"
is_template = true
hosts = ["macbook-pro"]

[shell.aliases.blog]
command = "cd ~/projects/blog && hugo server -D"
hosts = ["macbook-pro"]

[repos.blog]
url = "https://github.com/you/blog.git"
target = "~/projects/blog"
hosts = ["macbook-pro"]

# --- Work machine only ---

[dotfiles.gitconfig_work]
source = "git/.gitconfig-work.tmpl"
target = "~/.gitconfig"
is_template = true
hosts = ["work-laptop"]

[shell.aliases.vpn]
command = "sudo openconnect --protocol=anyconnect vpn.company.com"
hosts = ["work-laptop"]

[shell.aliases.k]
command = "kubectl"
hosts = ["work-laptop"]
[shell.aliases.kprod]
command = "kubectl --context=prod"
hosts = ["work-laptop"]
[shell.aliases.kstg]
command = "kubectl --context=staging"
hosts = ["work-laptop"]

[repos.work_infra]
url = "git@github.com:company/infra.git"
target = "~/work/infra"
hosts = ["work-laptop"]

[repos.work_services]
url = "git@github.com:company/services.git"
target = "~/work/services"
hosts = ["work-laptop"]

[hooks.builds.work_tools]
commands = ["make install"]
working_dir = "~/work/infra/tools"
run = "once"
hosts = ["work-laptop"]

# --- Server only ---

[dotfiles.tmux_server]
source = "tmux/.tmux-server.conf"
target = "~/.tmux.conf"
hosts = ["prod-server-01", "prod-server-02"]

[[tools]]
name = "docker"
check_command = "command -v docker"
install_hint = "https://docs.docker.com/engine/install/"
hosts = ["work-laptop", "prod-server-01", "prod-server-02"]
```

### The recipe-based setup

Once your config gets big enough, you stop wanting it in one file. Recipes let you split things up by topic -- each piece lives next to the files it manages.

Your dotfiles repo would look like this:

```
~/.dotfiles/
  config.toml              # main config, references recipes
  editors/
    recipe.toml            # editor configs
    nvim/                  # nvim config directory
    ideavimrc              # JetBrains vim bindings
  terminals/
    recipe.toml            # terminal emulator configs
    kitty/kitty.conf
    alacritty/alacritty.yml
  git/
    recipe.toml            # git configs
    .gitconfig.tmpl
    .gitignore_global
  kubernetes/
    recipe.toml            # k8s stuff (work machine only)
    kubeconfig-helpers.sh
```

The main config stays clean:

```toml
# ~/.dotfiles/config.toml
dotfiles_repo_path = "~/.dotfiles"

[template_variables]
git_name = "Your Name"
git_email = "you@example.com"

# Load recipes explicitly
[[recipes]]
path = "editors/recipe.toml"

[[recipes]]
path = "terminals/recipe.toml"

[[recipes]]
path = "git/recipe.toml"

[[recipes]]
path = "kubernetes/recipe.toml"
hosts = ["work-laptop"]          # only on work machine

# Shared shell config that doesn't belong to any recipe
[shell.functions.mkcd]
body = '''
  mkdir -p "$1" && cd "$1"
'''
```

And each recipe handles its own domain:

```toml
# editors/recipe.toml
[recipe]
name = "editors"
description = "Editor configurations"

[dotfiles.nvim_config]
source = "nvim"
target = "~/.config/nvim"
action = "symlink_dir"

[dotfiles.ideavimrc]
source = "ideavimrc"
target = "~/.ideavimrc"

[shell.aliases.vim]
command = "nvim"
[shell.aliases.vi]
command = "nvim"

[[tools]]
name = "nvim"
check_command = "command -v nvim"
install_hint = "https://neovim.io/"
```

```toml
# git/recipe.toml
[recipe]
name = "git"
description = "Git configuration and global ignores"

[dotfiles.gitconfig]
source = ".gitconfig.tmpl"
target = "~/.gitconfig"
is_template = true

[dotfiles.gitignore_global]
source = ".gitignore_global"
target = "~/.gitignore_global"

[shell.aliases.g]
command = "git"
[shell.aliases.gs]
command = "git status -sb"
[shell.aliases.gd]
command = "git diff"
[shell.aliases.gl]
command = "git log --oneline --graph --decorate -20"
```

```toml
# kubernetes/recipe.toml
[recipe]
name = "kubernetes"
description = "Kubernetes tools and aliases"

[shell.aliases.k]
command = "kubectl"
[shell.aliases.kgp]
command = "kubectl get pods"
[shell.aliases.kgs]
command = "kubectl get svc"
[shell.aliases.klog]
command = "kubectl logs -f"

[shell.functions.kexec]
body = '''
  kubectl exec -it "$1" -- /bin/sh
'''

[[tools]]
name = "kubectl"
check_command = "command -v kubectl"
install_hint = "https://kubernetes.io/docs/tasks/tools/"

[[tools]]
name = "kubectx"
check_command = "command -v kubectx"
install_hint = "https://github.com/ahmetb/kubectx"
```

If you prefer auto-discovery instead of listing each recipe, replace the `[[recipes]]` entries with:

```toml
[recipes_config]
auto_discover = true
exclude = ["experimental/*"]

[recipes_config.overrides.kubernetes]
hosts = ["work-laptop"]
```

Ralph will find every `recipe.toml` in your dotfiles repo and load them all. The `overrides` section lets you apply host filtering or disable specific recipes without touching the recipe files themselves.

## Best practices

1. **Version control your dotfiles.** Keep the source files in a git repo (e.g., `~/.dotfiles`).
2. **Version control `config.toml` too.** Put it in the same repo.
3. **Symlink `config.toml`** to `~/.config/ralph/config.toml`:
   ```bash
   ln -s ~/.dotfiles/config.toml ~/.config/ralph/config.toml
   ```
4. Run `ralph apply` after any changes to your config or dotfiles.

## Using `pkg/pipeutil` for Custom Shell Binaries

ralph includes a utility package `github.com/mad01/ralph/pkg/pipeutil` for writing Go programs that work as shell filters or transformers.

**Functions:**
- `pipeutil.ReadAll()`: Read all of stdin
- `pipeutil.Scanner()`: Line-by-line scanner for stdin
- `pipeutil.Print([]byte)`: Write to stdout
- `pipeutil.Println(string)`: Write string + newline to stdout
- `pipeutil.Error(error)` / `pipeutil.Errorf(format, ...)`: Write to stderr
- `pipeutil.ExitSuccess` / `pipeutil.ExitFailure`: Exit code constants

**Example:**

```go
package main

import (
	"os"
	"strings"

	"github.com/mad01/ralph/pkg/pipeutil"
)

func main() {
	binput, err := pipeutil.ReadAll()
	if err != nil {
		pipeutil.Errorf("failed to read from stdin: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}

	upper := strings.ToUpper(string(binput))

	_, err = pipeutil.Println(upper)
	if err != nil {
		pipeutil.Errorf("failed to write to stdout: %v", err)
		os.Exit(pipeutil.ExitFailure)
	}
	os.Exit(pipeutil.ExitSuccess)
}
```

```bash
go build -o mytool
echo "some input" | ./mytool
```

## A word from Ralph Wiggum

> *"My cat's breath smells like cat food."*
> — Ralph Wiggum

Ralph once tried to manage his dotfiles by hand. He opened `.bashrc`, typed `alias cat="dog"`, and waited for a puppy to come out. When nothing happened, he added `sudo` in front. Then he `rm -rf`'d his home directory because he wanted "a fresh start, like summer."

Chief Wiggum found him two hours later, staring at a kernel panic, whispering *"Go, banana!"* at a Go compiler error. Miss Hoover said the computer had "the hiccups." The IT guy quit.

That's why this tool exists. Ralph can't be trusted with a terminal unsupervised, but he *can* be trusted to `ralph apply` a config file someone else wrote for him.

---

### Ralph's favorite aliases

These are actually useful. Ralph just named them.

```toml
# "I'm learnding!" - typo aliases for fat fingers
[shell.aliases.sl]
command = "ls"        # you typed sl, you meant ls, we both know it

[shell.aliases.gti]
command = "git"       # vroom vroom

[shell.aliases.claer]
command = "clear"     # ironic

[shell.aliases.pdw]
command = "pwd"       # close enough

[shell.aliases.gerp]
command = "grep"      # gerp gerp gerp

# "Me fail English? That's unpossible!"
[shell.aliases.yolo]
command = "git push --force-with-lease"  # at least use the seatbelt version

[shell.aliases.oops]
command = "git reset --soft HEAD~1"      # undo last commit, keep changes

[shell.aliases.please]
command = "sudo !!"                      # "I'm a big kid now"

[shell.aliases.vanish]
command = "git stash"                    # now you see it...

[shell.aliases.unvanish]
command = "git stash pop"               # ...now you don't. wait, reverse that

# "When I grow up, I want to be a principal or a caterpillar"
[shell.aliases.ports]
command = "lsof -i -P -n | grep LISTEN"  # what's hogging my ports

[shell.aliases.weather]
command = "curl -s wttr.in/?format=3"    # Ralph checks the sky

[shell.aliases.path]
command = "echo $PATH | tr ':' '\\n'"    # readable PATH output

[shell.aliases.dotfiles]
command = "ralph apply --dry-run"         # see what ralph would do without doing it
```

### Ralph's shell functions

```toml
# "The doctor said I wouldn't have so many nosebleeds
#  if I kept my finger outta there."
#
# mkcd: make a directory and cd into it, because
# doing it in two commands is too many commands.
[shell.functions.mkcd]
body = '''
  mkdir -p "$1" && cd "$1"
'''

# "I bent my wookiee."
#
# gitignore: generate a .gitignore from gitignore.io
# Usage: gitignore go,vim,macos
[shell.functions.gitignore]
body = '''
  curl -sL "https://www.toptal.com/developers/gitignore/api/$1"
'''

# "It tastes like burning."
#
# nuke_modules: delete all node_modules directories.
# recursive. merciless. freeing.
[shell.functions.nuke_modules]
body = '''
  echo "Finding node_modules directories..."
  local count=$(find . -name "node_modules" -type d -prune | wc -l | tr -d ' ')
  if [ "$count" -eq 0 ]; then
    echo "No node_modules found. The burning has stopped."
    return 0
  fi
  echo "Found $count node_modules directories. Delete them all? [y/N]"
  read -r confirm
  if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
    find . -name "node_modules" -type d -prune -exec rm -rf {} +
    echo "Gone. Reduced to atoms."
  else
    echo "Ralph put the fire out."
  fi
'''
```

> *"I picked this tool and I'm not even crying!"*
> — Ralph, after his first successful `ralph apply`

## Contributing

Contributions welcome. Open an issue or PR on [GitHub](https://github.com/mad01/ralph).

## License

See [LICENSE](LICENSE) file.
