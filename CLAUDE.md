# CLAUDE.md - Ralph

A Go CLI tool for managing dotfiles and shell configurations. Uses a TOML config file to define symlinks, copies, aliases, functions, env vars, repos, build hooks, and packages. Named after Ralph Wiggum.

## Quick Reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Install | `make install` |
| Test | `make test` |
| Integration tests | `make test-integration` |
| Lint | `make lint` |
| Format | `make format` |
| Run | `./ralph up` |
| Apply only | `./ralph up --no-sync` |
| Check outdated | `./ralph outdated` |
| Install skills | `./ralph install-skills` |
| Sandbox | `make sandbox` |

## Architecture

```
cmd/ralph/
  main.go                    Thin entry point, calls commands.Execute()
  commands/
    root.go                  Cobra root command + global flags (--dry-run, --verbose, --quiet)
    cmd_up.go                ralph up - primary command (pull, sync, apply all)
    cmd_apply.go             ralph apply - apply configs (deprecated, use ralph up)
    cmd_init.go              ralph init - interactive config creation
    cmd_add.go               ralph add - scaffold a new recipe directory
    cmd_enable.go            ralph enable/disable - toggle recipe override
    cmd_list.go              ralph list - show managed items
    cmd_doctor.go            ralph doctor - health checks
    cmd_graph.go             ralph graph - render the recipe dependency DAG as wave layers
    cmd_clean.go             ralph clean - remove orphaned recipe artifacts (delete/abandon)
    cmd_state.go             ralph state - inspect the recipe artifact manifest
    cmd_sync.go              ralph sync - pull dotfiles repo and remote packages (deprecated, use ralph up)
    cmd_install_skills.go    ralph install-skills - install ralph's bundled Claude Code skills into ~/.claude/skills/
    cmd_migrate.go           ralph migrate - update broken symlinks (--status for plan preview)
    cmd_outdated.go          ralph outdated - check for newer versions of packages
    cmd_version.go           ralph version (-o json emits the four-field build object — cross-tool convention)

internal/
  binversion/
    binversion.go            Probe an installed binary via `<bin> version -o json` (cross-tool version convention)
  buildinfo/
    buildinfo.go             Link-time build metadata (version/commit/tag/build_time) + debug.ReadBuildInfo fallback
  config/
    types.go                 Config, Dotfile, Repo, Tool, Package, ShellConfig structs (TOML)
    load.go                  LoadConfig from XDG path
    validate.go              ValidateConfig, ValidateMergedConfig, ExpandPath
    deps.go                  Topological sort (Kahn's algorithm) for depends_on ordering
    enable.go                IsEnabled (*bool pattern: nil/true=enabled)
    host.go                  Host and profile filtering (ShouldApplyForHost, ShouldApplyForProfiles)
    recipe.go                Recipe loading, discovery, and merging
    sources.go               Remote recipe sources ([[recipe_sources]]): cache under ~/.config/ralph/sources/
    overrides.go             Set/remove recipe overrides in config.toml (text-level, backup+validate)
    vars.go                  Recipe variables: {{vars.<name>}} expansion in shell items ([recipe.vars] defaults + override values)
    migrate.go               MigrateFromLegacy (dotter → ralph)
  dotfile/
    symlink.go               Create/update symlinks and dir symlinks
    copy.go                  Copy files
    mkdir.go                 Create directories
    template.go              Go template processing
  shell/
    rc_manager.go            Manage .bashrc/.zshrc/config.fish (RALPH MANAGED BLOCK)
    functions.go             Generate aliases and functions shell scripts
  hooks/
    hooks.go                 Run lifecycle hooks (pre/post apply/link)
    builds.go                Build hooks with run modes (always/once/manual), subtree tree-hash freshness tracking
  repo/
    clone.go                 Git clone/pull/checkout via os/exec
  migrate/
    migrate.go               Symlink migration after repo reorganization
  report/
    report.go                Structured run reporting with phases and step results; JSON projection (ToJSON/WriteJSON) for --output json
  packages/
    update.go                SyncPackages (clone/pull) and BuildPackages (change detection, build, install)
    outdated.go              CheckOutdated for go-install, remote, and make packages
  skills/
    install.go               Install Claude Code skills from remote repos (discover, clone, symlink)
  tool/
    status.go                Tool check status via sh -c

```

## Conventions

- Config: TOML via `github.com/BurntSushi/toml`, lives at `~/.config/ralph/config.toml`
- CLI: `github.com/spf13/cobra`, each command in its own `cmd_*.go`, registered via `init()`
- Enable pattern: `*bool` field — nil/true = enabled, false = disabled
- Host filtering: `hosts` field on most items — empty = all hosts
- Profile filtering: `profiles` field on recipes and most items — empty = all profiles; a mismatched recipe freezes its artifacts, a mismatched item orphans them (cleanup removes)
- Recipes: modular `recipe.toml` files, auto-discovered or explicit references
- Git operations via `os/exec` in `internal/repo/`
- Dry-run: `--dry-run`/`-n` global flag, threaded through all operations; implies --verbose
- Build state tracked in `~/.config/ralph/.builds_state` (JSON), packages use `pkg:` prefix keys; idempotent builds also record a content hash there
- `run = "once"` builds and remote/make/local packages detect changes via the **subtree tree-hash** of `working_dir` (`git rev-parse HEAD:<subdir>`), not the repo-wide commit — so commits elsewhere in the repo don't force a rebuild; the scoped dirty check (`git status --porcelain -- .`) excludes gitignored build output. See `internal/gitutil` (`GetTreeHash`, `HasGitChangesInPath`).
- Build hooks support either inline commands or a script file (mutually exclusive)
- Exec timeouts: `timeout` field (seconds, default 600) on builds and packages; all exec.Command calls use context.WithTimeout
- Dependency ordering: `depends_on` on builds and packages; topological sort (Kahn's algorithm) determines execution order in a unified phase
- Package sources: `local`, `remote`, `make` (remote + default make build/install), `go-install` (go install module@version)
- Recipe artifact manifest tracked in `~/.config/ralph/.recipe_state` (JSON); written on **every** successful `ralph up`/`apply` (not only under `--enable-cleanup`, so the cleanup baseline never goes stale — `recordManifestAndCleanup`), consumed by the cleanup phase, inspectable via `ralph state show`
- Cleanup safety invariants (SafeRemove): never deletes any path (symlink, copy, dir, or `install_path`) still declared by an active recipe in the intended manifest (cross-recipe guard via `RecipeState.AllPaths`, computed before any `--recipe` filtering), never deletes the running `ralph` binary or a symlink to it (`ErrSelfDelete`, via `os.Executable()`); package clone dirs and repos are abandon-with-log, never auto-removed
- Cleanup lifecycle invariants: uninstall hooks (`pre/post_uninstall`) run only when a recipe is removed **entirely** — a partial orphan (recipe still present, one artifact dropped) does not fire them; a path whose removal fails is re-tracked (`RecipeState.MergeRetry`, hooks dropped) so the next run retries instead of leaking
- Self-healing apply: a missing declared `install_path` forces a package rebuild on a normal `ralph up` (`firstMissingInstallPath`), so a deleted binary recovers without `--reset-builds`
- Packages: `[packages]` config section — `ralph up` pulls and builds in one step
- Recipe source profiles gate the entire remote source before checkout, discovery, sync, and fingerprinting; `.recipe_state` records each remote recipe's source so cleanup freezes by provenance rather than by a name prefix. Version-0 state is migrated once using the exact `<source>/` namespace, conservatively preserving ambiguous legacy entries.
- Package clone dir: `packages_dir` config field (default: `~/.config/ralph/pkg/`)
- Generated shell scripts in `~/.config/ralph/generated/` (generated_aliases.sh, generated_functions.sh, generated_env.sh)
- Build metadata embedded via `-ldflags` into `internal/buildinfo` (version, commit, tag, build_time); `debug.ReadBuildInfo()` fills whatever the linker did not set
- Integration tests run in Docker containers (`tests/integration/`)

## Apply Execution Order

1. Legacy migration (dotter → ralph config)
2. Pre-apply hooks
3. Directories
4. Directory mirrors (`dirs_mirror` entries — symlink files or subdirectories from source to target)
5. Repositories (clone/update)
6. Dotfiles (symlink/copy/template)
7. Shell configuration (generate alias+function files, inject source lines)
8. Tool checks
9. Builds + Packages (unified phase, topologically sorted by `depends_on`, interleaved)
10. Post-apply hooks
11. Cleanup (if `--enable-cleanup` or `auto_cleanup`)
12. Print report summary

## Documentation

```
docs/
  getting-started.md     Zero to working setup
  commands.md            All commands with flags and examples
  configuration.md       Full config.toml schema reference
  recipes.md             Modular configuration with auto-discovery
  packages.md            Package management (ralph up)
  workflows.md           Daily usage patterns (up/disable+cleanup/doctor)
  templating.md          Go template system
  migration.md           Symlink migration after reorganization

examples/
  minimal/               Simplest setup (3 dotfiles, 2 aliases)
  recipe-based/          Modular config with auto-discovery
  with-packages/         Local and remote package builds
  dotfiles-repo/         Full dotfiles repository structure
```

## Key Files

- `config.toml` — user's dotfiles configuration (at `~/.config/ralph/config.toml`)
- `docs/` — user-facing documentation (guides and reference)
- `examples/` — example configurations and dotfiles repositories
- `configs/examples/` — default config templates (used by `ralph init`)
- `tests/integration/` — Docker-based integration test scripts

## Working on ralph itself

### Build loop

```bash
cd ~/code/src/github.com/mad01/ralph
make build          # produces ./ralph in the repo root
make install        # build + copy to GOBIN ($(go env GOBIN) or $HOME/go/bin)
make lint           # golangci-lint
make format         # goimports + gofmt
```

Build embeds four values into `internal/buildinfo` via `-ldflags`: the short commit as the version string, the full commit, the last tag (`git describe --tags --abbrev=0`), and the build time. Each is optional — outside a git checkout the Makefile passes an empty value and `buildinfo.Get()` falls back to what the Go toolchain stamped in.

`ralph version` prints the version string alone. `ralph version -o json` prints all four:

```json
{
  "version": "2917e73",
  "commit": "2917e735a634884fa21ff45a833e2067dc2236be",
  "tag": "v0.1.0",
  "build_time": "2026-08-13T19:32:27Z"
}
```

This shape is the cross-tool convention (`internal/binversion` probes sibling tools for it, `ralph doctor` annotates installed binaries with it). Keys are always present, unknown values are empty, and tools predating the four-field object report `version` alone.

### Running ralph safely against a real dotfiles repo

Use `--dry-run` (`-n`) to preview without writing anything. Dry-run implies verbose, so every item is printed:

```bash
./ralph up --dry-run
```

To test against a scratch config (isolate from your real setup):

```bash
mkdir -p /tmp/test-dots
cat > /tmp/ralph-test.toml << 'EOF'
dotfiles_repo_path = "/tmp/test-dots"
[dotfiles.test]
source = "test.txt"
target = "/tmp/test-target.txt"
EOF
touch /tmp/test-dots/test.txt
RALPH_CONFIG=/tmp/ralph-test.toml ./ralph up --dry-run
```

ralph reads config from `RALPH_CONFIG` env var or `~/.config/ralph/config.toml`.

### Where state lives

| Path | Contents |
|---|---|
| `~/.config/ralph/config.toml` | User config (or `RALPH_CONFIG`) |
| `~/.config/ralph/.builds_state` | Build + package change-detection hashes (JSON) |
| `~/.config/ralph/.recipe_state` | Recipe artifact manifest — what each recipe owns (JSON); drives cleanup |
| `~/.config/ralph/generated/` | Generated shell files: `generated_aliases.sh`, `generated_functions.sh`, `generated_env.sh` |
| `~/.config/ralph/pkg/` | Default clone dir for remote/make packages (`packages_dir`) |
| `~/.config/ralph/sources/` | Cached checkouts of remote recipe sources (`[[recipe_sources]]`) |

`ralph state show` prints `.recipe_state` in a readable form.

### How waves and depends_on resolve

Apply step 9 (builds + packages) is a unified phase. Ralph topologically sorts all items using Kahn's algorithm (`internal/config/deps.go`). Within the same wave, items with no dependency edge maintain alphabetical order. Waves partition the sort: wave-0 items all complete before wave-1 items run.

`ralph graph` renders the DAG as wave layers — useful for debugging ordering.

### Running tests

Unit tests (no external dependencies):

```bash
make test                        # go test ./... with 30s timeout
```

Integration tests run in Docker containers (require Docker):

```bash
make test-integration            # all suites
make test-integration-basic      # one suite
make sandbox                     # interactive Docker container for manual exploration
```

Integration tests spin up a container with a real ralph binary and a scratch dotfiles repo — they don't touch the host filesystem. Individual suites are shell scripts under `tests/integration/test_*/run_test.sh`.

### Reconciler architecture

The apply loop in `cmd_up.go` calls `commands.Apply()` which iterates the merged config in phases (see Apply Execution Order above). Each phase is a loop over the relevant config items, each item returning a `report.StepResult`. The report collects all results and prints a summary at the end.

The cleanup phase (`--enable-cleanup`) is a diff between:
- the current run's intended manifest (all artifacts the active config would produce), and
- the previous run's recorded manifest from `.recipe_state`.

Items in the recorded manifest that aren't in the intended manifest are orphans. `SafeRemove` deletes them with safety rails: checks the path is still not claimed by any active recipe (`RecipeState.AllPaths`), refuses to delete the running ralph binary (`ErrSelfDelete`), and refuses to delete outside HOME-prefixed paths. Package clone dirs and repos are abandoned (logged, not removed).

### Adding a new recipe-declaration type

1. Add the struct to `internal/config/types.go` (e.g. `MyThing struct { ... }`).
2. Add a map field to `Config` and/or `Recipe` (e.g. `MyThings map[string]MyThing`).
3. Implement the apply logic as a new phase function. Follow the pattern of existing phases: iterate the map, return `[]report.StepResult`, handle dry-run.
4. Wire it into the phase sequence in `cmd_up.go` (and `cmd_apply.go` for backward compat).
5. Add a `ralph list` subcommand or extend the existing one if the type should be listed.
6. Write a unit test and, if state-bearing, an integration test.

### Debugging a failed apply

1. Run with `--verbose` or `--dry-run` to see per-item output.
2. Check `ralph doctor` — it catches broken symlinks, missing tools, uncloned remotes, and bad build state.
3. Inspect build state: `ralph state show` lists what each recipe owns and when it last built.
4. Force a rebuild: `ralph up --force` re-runs every package and `once` build regardless of hash.
5. Clear all build state: `ralph up --reset-builds` (not needed to recover a deleted binary — a missing `install_path` self-heals on a plain `ralph up`).

### Known hazard: never combine --no-sync with --enable-cleanup

`--no-sync` skips pulling remote packages. If a remote package binary is missing from disk and the package hasn't been synced, `--enable-cleanup` sees the binary as an orphan and removes it — including the `ralph` binary itself if it is managed as a package. Recovery: `ralph up --reset-builds` (from a copy of the binary elsewhere in PATH, or reinstall via `go install`).
