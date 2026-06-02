# Commands Reference

Ralph provides commands for managing dotfiles, shell configurations, packages, and system health checks.

## Global Flags

These flags are available on all commands:

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--dry-run` | `-n` | `false` | Show what changes would be made without applying them. Implies --verbose. |
| `--verbose` | `-v` | `false` | Show per-item detail in the summary. Without this flag, only phase count lines are printed. |
| `--quiet` | `-q` | `false` | Show only failures in the summary. |
| `--output` | `-o` | `text` | Output format: `text` (human-readable) or `json` (machine-readable). |

### JSON output

Pass `--output json` (`-o json`) to the report-producing commands (`up`,
`doctor`, `clean`, and the deprecated `apply`/`sync`) to emit a stable,
machine-readable run report on stdout instead of the human summary. Progress
and log lines are suppressed in this mode so stdout carries only the JSON
document — pipe it straight into `jq`:

```bash
ralph up --no-sync -o json | jq '.summary'
# { "ok": 12, "warnings": 0, "failed": 0, "skipped": 1 }

ralph doctor -o json | jq -e '.summary.failed == 0'   # exit 0 when healthy
```

The document shape is:

```json
{
  "command": "up",
  "dry_run": false,
  "summary": { "ok": 0, "warnings": 0, "failed": 0, "skipped": 0 },
  "phases": [
    { "name": "Dotfiles", "steps": [
      { "name": "bashrc", "status": "ok", "message": "", "recipe": "" }
    ] }
  ],
  "exit_code": 0
}
```

`status` is one of `ok`, `warn`, `fail`, `skip`; `error` is added to a step only
when it failed. `exit_code` matches the process exit code (0 clean, 1 failures,
2 warnings-only). This is the contract the integration tests assert against.

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
| `--reset-builds` | `false` | Clear all build state before running, forcing every package and `once` build to rebuild. Not needed to recover a deleted binary — a missing `install_path` self-heals on a plain `ralph up` (see [packages › change detection](packages.md#change-detection)). |
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

## Uninstalling a recipe

There is no dedicated uninstall command. Uninstalling is declarative: disable
the recipe, then let `ralph up` reconcile the difference and prune the orphaned
artifacts.

```bash
# 1. Disable the recipe (writes enable = false to config.toml)
ralph disable my-recipe

# 2. Reconcile: up removes artifacts no longer owned by an enabled recipe
ralph up --enable-cleanup
```

If the recipe defined `pre_uninstall` / `post_uninstall` hooks, they run during
this cleanup (before and after artifact removal) to tear down external state
ralph doesn't track — see [`[hooks.pre_uninstall]`](configuration.md#hookspre_uninstall-and-hookspost_uninstall).

The cleanup phase compares the artifact manifest from the previous run against
the set of artifacts the now-reduced config intends to manage. Anything the
disabled recipe used to own — symlinks, copies, directories, and shell
aliases/functions/env vars — becomes an orphan and is removed through the same
safe-delete rails the cleanup phase always uses (HOME-prefixed paths only, no
globs, kind-checked). Preview it first with `--dry-run`:

```bash
ralph up --enable-cleanup --dry-run
```

To remove the recipe permanently, delete its directory from the dotfiles repo
(or its `[[recipes]]` reference) and run `ralph up --enable-cleanup` — the same
reconcile removes its artifacts. Set `auto_cleanup = true` under
`[recipes_config]` to make every `ralph up` reconcile without the flag.

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

These commands only change the config override -- they do not install or remove artifacts. Use `ralph up` after enabling to apply, or `ralph up --enable-cleanup` after disabling to also remove the recipe's artifacts.

### Flags

No additional flags.

### Examples

```bash
# Enable a recipe
ralph enable my-recipe

# Disable a recipe (config only, no cleanup)
ralph disable my-recipe

# Disable and remove its artifacts
ralph disable my-recipe && ralph up --enable-cleanup
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
| `--reset-builds` | `false` | Clear all build state before running, forcing every package and `once` build to rebuild. Not needed to recover a deleted binary — a missing `install_path` self-heals on a plain `ralph up` (see [packages › change detection](packages.md#change-detection)). |
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

Use `ralph up --enable-cleanup` to run cleanup as part of an apply. Use `ralph clean` to run it standalone — for example, when you have already applied and only want to prune.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--recipe=NAME` | `""` | Clean only the named recipe. The recipe must have entries in the previous manifest. State is not persisted under this flag — other recipes' entries are left untouched. |

Honors the global `--dry-run` flag.

### Safety rails

Every removal goes through `SafeRemove`, which rejects:

- Paths containing glob characters (`*`, `?`, `[`, `]`, `{`, `}`)
- Paths outside `$HOME`
- `install_paths` still declared by an active package or build in the current manifest — never removed, even if the recipe that previously owned them is now an orphan (cross-recipe guard)
- The currently-running `ralph` binary, or a symlink pointing to it — cleanup never deletes itself mid-run
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

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Show all items, not just problems. Equivalent to `--verbose` for doctor output. |

### Examples

```bash
# Run all health checks
ralph doctor

# Show all items including healthy ones
ralph doctor --all

# --verbose also shows all items
ralph doctor --verbose
```

## `ralph sync` (deprecated)

> **Deprecated:** Use `ralph up` instead, which syncs and applies in one step.

Pulls the dotfiles repository and clones or pulls remote packages. Does not build or install anything -- use `ralph up` instead, which handles both syncing and building.

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

## `ralph graph`

Renders the recipe dependency graph as horizontal wave layers, showing which recipes run in each wave and a summary of their builds and packages.

### Flags

No additional flags.

### Examples

```bash
ralph graph
```

## `ralph install-skills`

Installs ralph's bundled Claude Code skills into `~/.claude/skills/`. Skills help Claude understand how to work with ralph configurations, recipes, and troubleshooting.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Overwrite existing skill symlinks or directories. |

### Examples

```bash
# Install skills
ralph install-skills

# Overwrite any existing skill files
ralph install-skills --force

# Preview without installing
ralph install-skills --dry-run
```

## `ralph version`

Prints the ralph version string. The version is the git commit hash embedded at build time via `-ldflags`.

With `-o json` it prints `{"version":"<sha>"}` on a single line. This is a
deliberate cross-tool convention: a tool exposes a `version` subcommand that,
under `-o json`, reports the commit it was built from in this exact shape, so a
single probe can ask any such tool what build it is. `ralph doctor` uses it to
annotate built binaries with their reported version (informational only — see
note below).

### Flags

No additional flags (honors the global `--output`).

### Examples

```bash
ralph version
# v1.2.3-g8c9aeb9

ralph version -o json
# {"version":"8c9aeb9"}

ralph version -o json | jq -r .version
```

> Note: the reported version is the *commit* a binary was built from, which is
> useful as build identity. It is intentionally not used to decide whether a
> build is stale — freshness is keyed on the `working_dir` subtree tree hash
> (see [packages](packages.md)), so unrelated commits in the same repo don't
> count as drift.
