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
- `ralph doctor` - Health check the setup
- `ralph list` - Show managed items and their status
- `ralph init` - Create a new config interactively
- `ralph migrate` - Update symlinks after repo reorganization
- `ralph install-skills` - Install ralph's Claude Code skills
- `ralph apply` - Apply configs (deprecated, use `ralph up`)
- `ralph sync` - Pull dotfiles repo and remote packages (deprecated, use `ralph up`)

## Config Location

Config lives at `~/.config/ralph/config.toml` (or `$XDG_CONFIG_HOME/ralph/config.toml`).

## Config Structure

```toml
dotfiles_repo_path = "~/.dotfiles"

[dotfiles.bashrc]
source = ".bashrc"        # Relative to dotfiles_repo_path
target = "~/.bashrc"      # Absolute path on system
action = "symlink"        # symlink (default), copy, symlink_dir
is_template = false       # Process as Go template
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
source = "remote"         # local or remote
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

1. Dotfiles repo pull
2. Remote package sync
3. Legacy migration
4. Pre-apply hooks
5. Directories
6. Repositories
7. Dotfiles (symlink/copy/template)
8. Shell configuration
9. Tool checks
10. Build hooks
11. Packages
12. Post-apply hooks
