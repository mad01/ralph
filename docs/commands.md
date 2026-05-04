# Commands Reference

Ralph provides commands for managing dotfiles, shell configurations, packages, and system health checks.

## Global Flags

These flags are available on all commands:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | `-n` | `false` | Show what changes would be made without applying them. |
| `--verbose` | `-v` | `false` | Show all items in the summary, including OK and skipped items. |
| `--quiet` | `-q` | `false` | Show only failures in the summary. |

## `ralph apply`

Applies all configurations defined in your config file. This is the primary command that processes dotfiles, shell settings, repositories, hooks, and more.

### Execution Order

1. Legacy migration (automatic dotter-to-ralph config migration)
2. Pre-apply hooks
3. Directories (create if missing)
4. Repositories (clone or pull)
5. Dotfiles (symlink, copy, or template)
6. Shell configuration (generate alias and function files, inject source lines into RC files)
7. Tool checks (verify installed, print hints for missing tools)
8. Build hooks
9. Packages (change detection, build, install)
10. Post-apply hooks
11. Print report summary

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--overwrite` | `false` | Overwrite existing files at symlink target locations. |
| `--skip` | `false` | Skip symlinking if the target file already exists. |
| `--force` | `false` | Force re-run of `once` builds and package rebuilds even if previously completed. |
| `--build=NAME` | `""` | Run only the named build hook. Also works with `manual` builds. |
| `--reset-builds` | `false` | Clear all build state before running. |
| `--enable-cleanup` | `false` | After apply, remove orphaned artifacts owned by recipes that disappeared or are disabled. Honors per-recipe `delete_behavior`. See [recipes](recipes.md#recipe-deletion-and-cleanup). |

If neither `--overwrite` nor `--skip` is provided, existing files are backed up before being replaced.

### Examples

```bash
# Standard apply
ralph apply

# Preview changes without modifying anything
ralph apply --dry-run

# Overwrite existing files instead of backing them up
ralph apply --overwrite

# Re-run a build that was previously completed
ralph apply --force

# Run a specific manual build
ralph apply --build=my-tool

# Reset all build state and apply
ralph apply --reset-builds

# Apply and prune orphans from recipes that were removed or disabled
ralph apply --enable-cleanup
```

## `ralph clean`

Removes artifacts owned by recipes that are gone from your config or disabled. Compares the recorded recipe manifest at `~/.config/ralph/.recipe_state` against the manifest the current config would produce, then removes the difference through the safe-delete rails.

Use `ralph apply --enable-cleanup` to run cleanup as part of an apply. Use `ralph clean` to run it standalone — for example, when you have already applied and only want to prune.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--recipe=NAME` | `""` | Clean only the named recipe. The recipe must have entries in the previous manifest. State is not persisted under this flag — other recipes' entries are left untouched. |

Honors the global `--dry-run` flag.

### Safety rails

Every removal goes through `SafeRemove`, which rejects:

- Paths containing glob characters (`*`, `?`, `[`, `]`, `{`, `}`)
- Paths outside `$HOME`
- Symlinks that are no longer symlinks (replaced by a regular file)
- Directories that are not empty
- Repos — always abandoned in v1, never auto-removed

Recipes flagged `delete_behavior = "abandon"` are logged but never have their artifacts removed.

### Examples

```bash
# Preview what cleanup would remove
ralph clean --dry-run

# Clean orphans across all recipes
ralph clean

# Test removal of a specific recipe without persisting state
ralph clean --recipe themes --dry-run
```

## `ralph state`

Inspects ralph's per-recipe artifact manifest at `~/.config/ralph/.recipe_state`.

### `ralph state show`

Prints the manifest. By default, formats as a human-readable list grouped by recipe and artifact kind.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output the raw manifest as indented JSON. |

### Examples

```bash
# Print the manifest
ralph state show

# Pipe the JSON form into jq
ralph state show --json | jq '.recipes.brain'
```

## `ralph init`

Interactively creates a new ralph configuration file. Prompts for the path to your dotfiles repository and writes a starter config to the default location.

If a config file already exists, you are asked whether to overwrite it.

### Flags

No additional flags.

### Examples

```bash
ralph init
```

## `ralph list`

Displays all items managed by ralph along with their current status.

Shows:
- Dotfiles and their symlink status (correctly linked, broken, not linked)
- Packages with update status, source type, and last build time
- Tools and their install status
- Shell aliases and their commands
- Shell functions

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | `""` | Filter packages by source type: `"local"` or `"remote"`. |

### Examples

```bash
# List everything
ralph list

# Show only remote packages
ralph list --source remote
```

## `ralph doctor`

Performs health checks across your entire ralph setup and reports any issues found.

Checks performed:
- Configuration file readability and validity
- Dotfile symlinks (broken links, wrong targets, non-symlink files)
- Directories (existence, correct type)
- Repositories (cloned, valid git repo)
- Build state (completed, pending, working directory existence)
- Packages (cloned, build state, working directory existence)
- Tools (installed or missing)
- RC files (ralph managed block present, sourced files exist)

### Flags

No additional flags.

### Examples

```bash
# Run all health checks
ralph doctor

# Show all items including healthy ones
ralph doctor --verbose
```

## `ralph sync`

Pulls the dotfiles repository and clones or pulls remote packages. Does not build or install anything -- run `ralph apply` after syncing to build packages.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--package=NAME` | `""` | Sync only the named package. |
| `--no-pull` | `false` | Skip pulling the dotfiles repo before syncing packages. |

### Examples

```bash
# Sync everything (pull dotfiles + remote packages)
ralph sync

# Sync a single package
ralph sync --package neovim

# Sync without pulling dotfiles repo first
ralph sync --no-pull

# Preview what would change
ralph sync --dry-run

# Full workflow: sync then apply
ralph sync && ralph apply
```

## `ralph migrate`

Updates broken symlinks after reorganizing your dotfiles repository structure. Uses `legacy_paths` mappings defined in recipe files to find and fix symlinks that point to old locations.

For this command to work, your recipes must define legacy path mappings:

```toml
[recipe.legacy_paths]
"old/path/file.txt" = "new/path/file.txt"
```

### Flags

No additional flags (uses global `--dry-run`).

### Examples

```bash
# Preview what symlinks would be updated
ralph migrate --dry-run

# Apply symlink migrations
ralph migrate
```

See [configuration](configuration.md) for details on the recipe file format and `legacy_paths`.

## `ralph add`

Planned command for adding new items to ralph management interactively. Not yet implemented.

### Flags

No additional flags.

## `ralph version`

Prints the ralph version string. The version is the git commit hash embedded at build time via `-ldflags`.

### Flags

No additional flags.

### Examples

```bash
ralph version
```
