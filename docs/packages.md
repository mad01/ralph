# Packages

Ralph can manage packages -- local or remote projects that need building and installing. Use `ralph sync` to pull remote packages and `ralph apply` to build them.

## Package source types

**Remote packages** (`source = "remote"`) are cloned from a git URL. `ralph sync` clones or pulls the latest changes. `ralph apply` detects changes via git hash comparison and rebuilds if needed.

**Local packages** (`source = "local"`) already exist on disk. `ralph sync` skips them (nothing to pull). `ralph apply` checks the git hash and uncommitted changes, and rebuilds if anything differs from the last recorded state.

**Make packages** (`source = "make"`) behave like remote packages (git clone/pull) but default to `build = ["make build"]` and `install = ["make install"]` when those fields are omitted. Explicit `build` and `install` values override the defaults. Requires the `repo` field.

**Go-install packages** (`source = "go-install"`) install external Go binaries via `go install module@version`. They require `module` and `version` fields. `ralph sync` skips them (nothing to clone). Change detection compares the `version` string against the last recorded state rather than a git hash. GOBIN is set to the directory of the first `install_paths` entry.

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

### Make package

```toml
[packages.my-tool]
source = "make"
repo = "git@github.com:user/tool.git"
install_paths = ["~/code/bin/tool"]
```

When `build` and `install` are omitted, they default to `["make build"]` and `["make install"]`. You can override either:

```toml
[packages.custom-make]
source = "make"
repo = "https://github.com/user/project.git"
build = ["make release"]
install = ["make install PREFIX=$HOME/.local"]
install_paths = ["~/.local/bin/project"]
```

### Go-install package

```toml
[packages.github_mcp_server]
source = "go-install"
module = "github.com/github/github-mcp-server/cmd/github-mcp-server"
version = "v1.0.5"
install_paths = ["~/code/bin/github-mcp-server"]
```

The `module` field is the full Go module path passed to `go install`. The `version` field is the version tag (e.g. `v1.0.5`). Ralph runs `go install <module>@<version>` with GOBIN set to the directory of the first `install_paths` entry.

To update a go-install package, change the `version` field and run `ralph apply`. Ralph detects the version change and re-runs the install.

## Package fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | yes | `"local"`, `"remote"`, `"make"`, or `"go-install"` |
| `repo` | string | remote/make | Git URL to clone |
| `target` | string | no | Clone directory for remote/make packages (default: `<packages_dir>/<name>`) |
| `branch` | string | no | Branch to track (remote/make only) |
| `module` | string | go-install only | Go module path for `go install` |
| `version` | string | go-install only | Version tag for `go install` |
| `working_dir` | string | no | Directory for build/install commands (defaults to `target` for remote, required for local) |
| `build` | list | conditional | Build commands. Required for local/remote. Defaults to `["make build"]` for make. Not used by go-install. |
| `install` | list | no | Install commands. Defaults to `["make install"]` for make. Not used by go-install. |
| `timeout` | int | no | Maximum execution time in seconds (default: 600) |
| `depends_on` | list | no | Items that must complete first. Format: `"builds.<name>"` or `"packages.<name>"` |
| `install_paths` | list | no | Files this package writes to disk. For go-install, GOBIN is set to the directory of the first entry. |
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
- For each remote or make package: clone if missing, pull if exists
- Local and go-install packages are skipped (nothing to sync)

### Apply step

During `ralph apply`, builds and packages run in a unified phase, ordered by `depends_on` (topological sort). Within this phase:
- For each package, check the working directory exists (remote/make packages not yet cloned are skipped with a hint to run `ralph sync` first)
- For remote/make/local packages: compare the current git hash against the last recorded state
- For go-install packages: compare the `version` string against the last recorded state
- For local packages, also check for uncommitted changes
- If changes are detected (or `--force` is set), run build and install commands
- All commands are subject to the `timeout` limit (default 600 seconds)
- Save state after a successful build

## Change detection

Ralph detects whether a package needs rebuilding by comparing:

- **Git commit hash** -- (remote, make, local) The current HEAD hash vs the hash recorded after the last build.
- **Version string** -- (go-install) The configured `version` value vs the version recorded after the last install.
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

Global flags `--dry-run`, `--verbose`, and `--quiet` also apply. Use `--dry-run` to preview what would happen without making changes (dry-run implies verbose output).

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

## Dependency ordering

Packages and builds support a `depends_on` field. During `ralph apply`, all builds and packages run in a single unified phase, ordered by topological sort. Items with no dependencies maintain alphabetical order relative to each other.

```toml
[hooks.builds.generate_config]
commands = ["./gen-config.sh"]
working_dir = "~/code/my-tool"
run = "always"

[packages.my-tool]
source = "make"
repo = "git@github.com:user/tool.git"
install_paths = ["~/code/bin/tool"]
depends_on = ["builds.generate_config"]
```

Cross-type dependencies are allowed. A build can depend on a package, and a package can depend on a build. Circular dependencies and dangling references are rejected at config validation time.

## Timeouts

All build and install commands run with a timeout. The default is 600 seconds. Set the `timeout` field on a build or package to override:

```toml
[packages.large-project]
source = "remote"
repo = "https://github.com/user/large-project.git"
build = ["make -j8"]
timeout = 1800
```

If a command exceeds the timeout, it is killed and the build is marked as failed.

## Checking for updates

`ralph outdated` checks for newer versions of managed packages without building or installing anything.

| Source | Check method |
|--------|-------------|
| `go-install` | Runs `go list -m -json <module>@latest` and compares against the configured `version`. |
| `remote`, `make` | Runs `git ls-remote <repo> HEAD` and compares against the last recorded git hash. |
| `local` | Skipped (no remote to check). |

### Flags

| Flag | Description |
|------|-------------|
| `--json` | Output results as JSON for machine consumption. |

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

# Combine with sync to pull updates
ralph outdated && ralph sync && ralph apply
```

## Checking status

Use `ralph list` to see the current state of all managed packages, including whether they need a rebuild:

```bash
ralph list
ralph list --source=remote       # filter to remote packages only
ralph list --source=local        # filter to local packages only
ralph list --source=make         # filter to make packages only
ralph list --source=go-install   # filter to go-install packages only
```

Use `ralph doctor` to check package health. It verifies that working directories exist, remote packages are cloned, and reports the last build timestamp.
