# Packages

Ralph can manage packages -- local or remote projects that need building and installing -- and keep them up to date with `ralph update`.

## Remote vs local packages

**Remote packages** are cloned from a git URL. On update, ralph pulls the latest changes and rebuilds if the git hash changed.

**Local packages** already exist on disk. On update, ralph checks the git hash and whether there are uncommitted changes, and rebuilds if anything differs from the last recorded state.

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

## The update workflow

Running `ralph update` performs these steps:

1. **Pull dotfiles repo** -- Pulls the latest changes from your dotfiles repository (skip with `--no-pull`).
2. **Process each package:**
   - **Remote:** If not yet cloned, clone the repo. Otherwise, pull latest changes. Compare the git hash before and after the pull. If the hash changed (or `--force` is set), run build and install commands.
   - **Local:** Read the current git hash and check for uncommitted changes. Compare against the last recorded state. If the hash differs, there are uncommitted changes, or `--force` is set, run build and install commands.
3. **Save state** -- After a successful build, ralph records the git hash and timestamp in the build state file (`~/.config/ralph/.builds_state`) using a `pkg:<name>` key.

## Change detection

Ralph detects whether a package needs rebuilding by comparing:

- **Git commit hash** -- The current HEAD hash vs the hash recorded after the last build.
- **Uncommitted changes** -- For local packages, ralph also checks for uncommitted modifications in the working directory.
- **Missing state** -- If a package has never been built (no entry in the state file), it is always rebuilt.

## Flags

| Flag | Description |
|------|-------------|
| `--force` | Rebuild all packages regardless of change detection |
| `--package=NAME` | Update only the specified package |
| `--no-pull` | Skip pulling the dotfiles repo before updating |

Global flags `--dry-run`, `--verbose`, and `--quiet` also apply. Use `--dry-run` to preview what would happen without making changes.

```bash
# Update all packages
ralph update

# Update a single package
ralph update --package=my-tool

# Force rebuild everything
ralph update --force

# Preview without changes
ralph update --dry-run

# Skip dotfiles repo pull
ralph update --no-pull
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

Use `ralph list` to see the current state of all managed packages, including whether they need an update:

```bash
ralph list
ralph list --source=remote   # filter to remote packages only
ralph list --source=local    # filter to local packages only
```

Use `ralph doctor` to check package health. It verifies that working directories exist, remote packages are cloned, and reports the last build timestamp.
