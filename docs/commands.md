# Commands Reference

Ralph provides commands for managing dotfiles, shell configurations, packages, and system health checks.

## Global Flags

These flags are available on all commands:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | `-n` | `false` | Show what changes would be made without applying them. Implies --verbose. |
| `--verbose` | `-v` | `false` | Show per-item detail in the summary. Without this flag, only phase count lines are printed. |
| `--quiet` | `-q` | `false` | Show only failures in the summary. |

## `ralph up`

Pulls the dotfiles repo, syncs remote packages, and applies all configurations in one command. Replaces the separate `ralph sync` + `ralph apply` workflow.

### Execution Order

1. Legacy migration (automatic dotter-to-ralph config migration)
2. Dotfiles repo pull
3. Remote package sync
4. Pre-apply hooks
5. Directories (create if missing)
6. Directory mirrors (symlink files or subdirectories from source to target)
7. Repositories (clone or pull)
8. Dotfiles (symlink, copy, or template)
9. Shell configuration (generate alias and function files, inject source lines into RC files)
10. Tool checks (verify installed, print hints for missing tools)
11. Builds + Packages (unified phase, topologically sorted by `depends_on`)
12. Post-apply hooks
13. Cleanup (if `--enable-cleanup` or `auto_cleanup` is set)
14. Print report summary

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--no-sync` | `false` | Skip the sync step (repo pull + package sync) and only apply. Equivalent to the old `ralph apply`. |
| `--overwrite` | `false` | Overwrite existing files at symlink target locations. |
| `--skip` | `false` | Skip symlinking if the target file already exists. |
| `--force` | `false` | Force re-run of `once` builds and package rebuilds even if previously completed. |
| `--build=NAME` | `""` | Run only the named build hook. Also works with `manual` builds. |
| `--reset-builds` | `false` | Clear all build state before running. |
| `--enable-cleanup` | `false` | After apply, remove orphaned artifacts owned by recipes that disappeared or are disabled. Honors per-recipe `delete_behavior`. See [recipes](recipes.md#recipe-deletion-and-cleanup). |

If neither `--overwrite` nor `--skip` is provided, existing files are backed up before being replaced.

### Examples

```bash
# Full sync + apply
ralph up

# Preview changes without modifying anything
ralph up --dry-run

# Apply only (skip repo pull and package sync)
ralph up --no-sync

# Overwrite existing files instead of backing them up
ralph up --overwrite

# Re-run a build that was previously completed
ralph up --force

# Run a specific manual build
ralph up --build=my-tool

# Reset all build state and apply
ralph up --reset-builds

# Sync, apply, and prune orphans from removed/disabled recipes
ralph up --enable-cleanup
```

## `ralph down`

Uninstalls a recipe by removing all its tracked artifacts, regenerating shell config, cleaning build state, and disabling it in config.toml.

### What it does

1. Checks for dependent items in other recipes (dependency guard)
2. Prompts for confirmation (y/N)
3. Runs `pre_uninstall` hooks (if defined in the recipe)
4. Removes tracked artifacts (symlinks, copies, directories) via safe-delete rails
5. Regenerates shell config without the recipe's aliases, functions, and env vars
6. Resets build and package state entries owned by the recipe
7. Runs `post_uninstall` hooks (if defined in the recipe)
8. Removes the recipe from the artifact manifest
9. Sets `enable = false` for the recipe in config.toml

### Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--yes` | `-y` | `false` | Skip the confirmation prompt. |
| `--force` | | `false` | Bypass the dependency guard and continue past `pre_uninstall` hook failures. |

Honors the global `--dry-run` flag.

### Examples

```bash
# Uninstall a recipe (interactive confirmation)
ralph down my-recipe

# Skip confirmation
ralph down my-recipe --yes

# Preview what would be removed
ralph down my-recipe --dry-run

# Force removal even if other recipes depend on it
ralph down my-recipe --force --yes
```

## `ralph add`

Scaffolds a new recipe directory with a template `recipe.toml` under `<dotfiles_repo_path>/recipes/<name>/`.

Recipe names must be alphanumeric with hyphens only (no leading hyphen). Errors if the recipe directory already exists.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--description` | `""` | Set the recipe description in the generated `recipe.toml`. |

Honors the global `--dry-run` flag.

### Examples

```bash
# Create a new recipe scaffold
ralph add my-tool

# Create with a description
ralph add my-tool --description "My custom tool setup"

# Preview without creating files
ralph add my-tool --dry-run
```

## `ralph enable` / `ralph disable`

Toggle a recipe override in config.toml. Sets `enable = true` or `enable = false` in `[recipes_config.overrides.<name>]`.

These commands only change the config override -- they do not install or remove artifacts. Use `ralph up` after enabling to apply, or `ralph down` after disabling to also clean up artifacts.

### Flags

No additional flags.

### Examples

```bash
# Enable a recipe
ralph enable my-recipe

# Disable a recipe (config only, no cleanup)
ralph disable my-recipe

# Disable and also remove artifacts
ralph disable my-recipe && ralph down my-recipe --yes
```

## `ralph apply` (deprecated)

> **Deprecated:** Use `ralph up --no-sync` instead.

Applies all configurations defined in your config file. This is the primary command that processes dotfiles, shell settings, repositories, hooks, and more.

### Execution Order

1. Legacy migration (automatic dotter-to-ralph config migration)
2. Pre-apply hooks
3. Directories (create if missing)
4. Repositories (clone or pull)
5. Dotfiles (symlink, copy, or template)
6. Directory mirrors (symlink files or subdirectories from source to target)
7. Shell configuration (generate alias and function files, inject source lines into RC files)
8. Tool checks (verify installed, print hints for missing tools)
9. Builds + Packages (unified phase, topologically sorted by `depends_on`)
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
| `--source` | `""` | Filter packages by source type: `"local"`, `"remote"`, `"make"`, or `"go-install"`. |

### Examples

```bash
# List everything
ralph list

# Show only remote packages
ralph list --source remote
```

### `ralph list recipes`

Shows all discovered recipes with their enabled/disabled status and a summary of item counts (dotfiles, aliases, functions, builds, packages, etc.).

```bash
# List all recipes and their status
ralph list recipes
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

## `ralph sync` (deprecated)

> **Deprecated:** Use `ralph up` instead, which syncs and applies in one step.

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

## `ralph outdated`

Checks for newer versions of managed packages. Compares the current state against the latest available version for each package.

How each source type is checked:

- **go-install**: Queries `go list -m -json <module>@latest` and compares against the configured `version`.
- **remote**, **make**: Queries `git ls-remote <repo> HEAD` and compares against the last recorded git hash.
- **local**: Skipped (no remote to check).

Output is a colorized table by default.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output results as JSON for machine consumption. |

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | All packages are up to date. |
| 1 | One or more packages have updates available. |
| 2 | Errors occurred during checks. |

### Examples

```bash
# Check all packages for updates
ralph outdated

# Machine-readable output
ralph outdated --json

# Check then update
ralph outdated && ralph up
```

## `ralph migrate`

Updates broken symlinks after reorganizing your dotfiles repository structure. Uses `legacy_paths` mappings defined in recipe files to find and fix symlinks that point to old locations.

For this command to work, your recipes must define legacy path mappings:

```toml
[recipe.legacy_paths]
"old/path/file.txt" = "new/path/file.txt"
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--status` | `false` | Show migration status per recipe without making changes. Reports which legacy_paths blocks still have on-disk paths vs. which can be safely removed. |

### Examples

```bash
# Preview what symlinks would be updated
ralph migrate --dry-run

# Apply symlink migrations
ralph migrate

# Check migration status
ralph migrate --status
```

See [configuration](configuration.md) for details on the recipe file format and `legacy_paths`.

## `ralph install-skills`

Installs Claude Code skills from remote git repositories by cloning and symlinking into the skills directory.

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
