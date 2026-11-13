---
name: ralph
description: Work with ralph dotfiles manager - apply configs, manage dotfiles, shell setup, packages, and hooks. Use when editing ralph config.toml, running ralph commands, or troubleshooting dotfile management.
---

# ralph - Dotfiles Manager

Use this skill when working with ralph, a CLI dotfiles manager that uses TOML configuration.

## Core Commands

- `ralph up` - Primary command (pull dotfiles repo, sync remote packages, apply all)
- `ralph up --no-sync` - Apply only (skip pull and package sync)
- `ralph add <recipe>` - Scaffold a new recipe directory
- `ralph enable/disable <recipe>` - Toggle recipe override
- `ralph disable <recipe> && ralph up --enable-cleanup` - Uninstall a recipe (disable, then reconcile)
- `ralph list recipes` - Show all recipes and status
- `ralph clean` - Remove artifacts of removed or disabled recipes (`--recipe NAME` for one)
- `ralph state show` - Print the recipe artifact manifest (`~/.config/ralph/.recipe_state`)
- `ralph config` - Show the merged user config and its path (`--effective` adds recipes and gates)
- `ralph profile show` / `ralph profile set <name>...` - Read or write this machine's profiles
- `ralph graph` - Show recipes as wave layers (dependency order)
- `ralph outdated` - Check packages for newer versions (exit 1 when updates exist)
- `ralph doctor` - Health check the setup (exit 1 on failures, 2 on warnings only)
- `ralph list` - Show managed items and their status
- `ralph init` - Create a new config interactively
- `ralph migrate` - Update symlinks after repo reorganization
- `ralph install-skills` - Install ralph's Claude Code skills
- `ralph apply` - Apply configs (deprecated, use `ralph up`)
- `ralph sync` - Pull dotfiles repo and remote packages (deprecated, use `ralph up`)

`-o json` works on `up`, `apply`, `clean`, `doctor`, `config`, `version`, `state show` and `outdated`. Every other command rejects it.

## Config Location

Config resolves in this order: `$RALPH_CONFIG` (the file must exist), then `$XDG_CONFIG_HOME/ralph/config.toml`, then `~/.config/ralph/config.toml`. A git-ignored `config.local.toml` next to it holds machine-local keys such as `profiles` (written by `ralph profile set`).

`RALPH_CONFIG` moves only the config file. State (`.builds_state`, `.recipe_state`, `.lock`) always lives in `~/.config/ralph/`, so a scratch config still touches the real state unless `HOME` points elsewhere too.

## Config Structure

```toml
dotfiles_repo_path = "~/.dotfiles"

[dotfiles.bashrc]
source = ".bashrc"        # Relative to dotfiles_repo_path
target = "~/.bashrc"      # Absolute path on system
action = "symlink"        # symlink (default), copy, symlink_dir
is_template = false       # Process as Go template (requires action = "copy")
hosts = []                # Empty = all hosts
enable = true             # nil/true = enabled

[directories.config_dir]
target = "~/.config/myapp"

[repos.my_repo]
url = "https://github.com/user/repo.git"
target = "~/code/repo"
branch = "main"
update = true

[shell.aliases.ll]
command = "ls -alh"

[shell.functions.mkcd]
body = "mkdir -p \"$1\" && cd \"$1\""

[shell.env]
EDITOR = "nvim"

[hooks]
pre_apply = ["echo 'starting'"]
post_apply = ["echo 'done'"]

[hooks.builds.my_tool]
commands = ["make", "make install"]
working_dir = "~/code/my-tool"
run = "once"              # always, once, manual

[packages.my_pkg]
source = "remote"         # local, remote, make, or go-install
repo = "https://github.com/user/pkg.git"
build = ["make"]
install = ["make install"]
```

## Key Patterns

- **Enable pattern**: `*bool` field - nil/true = enabled, false = disabled
- **Host filtering**: `hosts = ["hostname"]` - empty means all hosts
- **Dry run**: `ralph up --dry-run` shows changes without applying
- **Build state**: Tracked in `~/.config/ralph/.builds_state` (JSON)
- **Generated scripts**: Written to `~/.config/ralph/generated/`
- **Recipes**: Modular config files in `recipes/<name>/recipe.toml`

## Apply Execution Order

1. Legacy migration (dotter to ralph config)
2. Pre-apply hooks
3. Directories
4. Directory mirrors (`dirs_mirror`)
5. Repositories
6. Dotfiles (symlink/copy/template)
7. Shell configuration
8. Tool checks
9. Builds + Packages (one phase, sorted by `depends_on`)
10. Post-apply hooks
11. Cleanup (with `--enable-cleanup` or `auto_cleanup`)
12. Report summary

`ralph up` syncs between steps 1 and 2: it pulls the dotfiles repo, recipe sources and remote packages. `--no-sync` skips that.
