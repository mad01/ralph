# Packages

Ralph can manage packages -- local or remote projects that need building and installing. Use `ralph sync` to pull remote packages and `ralph apply` to build them.

## Remote vs local packages

**Remote packages** are cloned from a git URL. `ralph sync` clones or pulls the latest changes. `ralph apply` detects changes via git hash comparison and rebuilds if needed.

**Local packages** already exist on disk. `ralph sync` skips them (nothing to pull). `ralph apply` checks the git hash and uncommitted changes, and rebuilds if anything differs from the last recorded state.

## Defining packages

Add packages to the `[packages]` section of your `config.toml` or a [recipe](recipes.md) file.

### Remote package

```toml
[packages.my-tool]
source = "remote"
repo = "https://github.com/user/tool.git"
branch = "main"
build = ["make"]
install = ["make install"]
```

### Local package

```toml
[packages.local-tool]
source = "local"
working_dir = "~/projects/my-tool"
build = ["go build -o mytool ."]
install = ["cp mytool ~/.local/bin/"]
```

## Package fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | yes | `"remote"` or `"local"` |
| `repo` | string | remote only | Git URL to clone |
| `target` | string | no | Clone directory for remote packages (default: `<packages_dir>/<name>`) |
| `branch` | string | no | Branch to track (remote only) |
| `working_dir` | string | no | Directory for build/install commands (defaults to `target` for remote, required for local) |
| `build` | list | yes | Commands to build the package |
| `install` | list | no | Commands to install after building |
| `hosts` | list | no | Hostnames this package applies to (empty = all hosts) |
| `enable` | bool | no | `nil`/`true` = enabled, `false` = disabled |

## Default paths

Remote packages are cloned to `<packages_dir>/<name>`. The `packages_dir` field in the top-level config defaults to `~/.config/ralph/pkg/`. You can override it:

```toml
packages_dir = "~/src/managed-packages"
```

If a remote package does not specify `working_dir`, it defaults to the clone `target` directory.

## The sync + apply workflow

Package management is split into two steps:

1. **`ralph sync`** -- Pulls the dotfiles repo and clones/pulls remote packages. No building.
2. **`ralph apply`** -- Detects changes and rebuilds packages as needed, alongside all other apply operations.

### Sync step

`ralph sync` performs:
- Pull the dotfiles repository (skip with `--no-pull`)
- For each remote package: clone if missing, pull if exists
- Local packages are skipped (nothing to sync)

### Apply step

During `ralph apply`, the Packages phase runs after build hooks:
- For each package, check the working directory exists (remote packages not yet cloned are skipped with a hint to run `ralph sync` first)
- Compare the current git hash against the last recorded state
- For local packages, also check for uncommitted changes
- If changes are detected (or `--force` is set), run build and install commands
- Save state after a successful build

## Change detection

Ralph detects whether a package needs rebuilding by comparing:

- **Git commit hash** -- The current HEAD hash vs the hash recorded after the last build.
- **Uncommitted changes** -- For local packages, ralph also checks for uncommitted modifications in the working directory.
- **Missing state** -- If a package has never been built (no entry in the state file), it is always rebuilt.

## Flags

### `ralph sync`

| Flag | Description |
|------|-------------|
| `--package=NAME` | Sync only the specified package |
| `--no-pull` | Skip pulling the dotfiles repo before syncing |

### `ralph apply`

| Flag | Description |
|------|-------------|
| `--force` | Rebuild all packages (and re-run `once` builds) regardless of change detection |

Global flags `--dry-run`, `--verbose`, and `--quiet` also apply. Use `--dry-run` to preview what would happen without making changes.

```bash
# Sync and apply (full workflow)
ralph sync && ralph apply

# Sync a single package
ralph sync --package=my-tool

# Force rebuild all packages
ralph apply --force

# Preview without changes
ralph sync --dry-run
ralph apply --dry-run
```

## Packages in recipes

Packages can be defined in [recipe](recipes.md) files exactly like in the main config:

```toml
# recipes/tools/recipe.toml
[recipe]
name = "tools"

[packages.my-tool]
source = "remote"
repo = "https://github.com/user/tool.git"
build = ["make"]
install = ["make install"]
```

Recipe-level `hosts` filtering applies to packages that do not define their own `hosts` field.

## Checking status

Use `ralph list` to see the current state of all managed packages, including whether they need a rebuild:

```bash
ralph list
ralph list --source=remote   # filter to remote packages only
ralph list --source=local    # filter to local packages only
```

Use `ralph doctor` to check package health. It verifies that working directories exist, remote packages are cloned, and reports the last build timestamp.
