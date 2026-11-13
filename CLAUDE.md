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
| Format check (CI) | `make format-check` |
| Install dev tools | `make install_deps` |
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
    root.go                  Cobra root command + global flags (--dry-run, --verbose, --quiet, --output) + the -o json gate
    common.go                Shared output helpers: verbose/UI writers, dry-run banner, report summary
    cmd_up.go                ralph up - primary command: sync sequence, then apply sequence
    cmd_apply.go             ralph apply - apply sequence only (deprecated, use ralph up --no-sync); the apply phase functions
    sequence.go              Shared run sequences: runApplyCommand (up/apply), runApplySequence, runSyncSequence
    cmd_init.go              ralph init - interactive config creation
    cmd_add.go               ralph add - scaffold a new recipe directory
    cmd_enable.go            ralph enable/disable - toggle recipe override
    cmd_list.go              ralph list - show managed items
    cmd_config.go            ralph config - show the merged user config (--effective: recipes and gates applied)
    cmd_profile.go           ralph profile show/set - this machine's profiles in config.local.toml
    cmd_doctor.go            ralph doctor - health checks
    doctor_dangling.go       Doctor check: dangling symlinks in directories recipes link into
    doctor_validate.go       Doctor check: dotfile target state per action (symlink, copy, symlink_dir)
    cmd_graph.go             ralph graph - render the recipe dependency DAG as wave layers
    cmd_clean.go             ralph clean - remove orphaned recipe artifacts (delete/abandon)
    cleanup.go               Cleanup phase: intended manifest, protected paths, frozen recipes, orphan removal
    cmd_state.go             ralph state - inspect the recipe artifact manifest
    cmd_sync.go              ralph sync - sync sequence only (deprecated, use ralph up)
    sources_sync.go          Pull update = true recipe sources; sync fingerprint for the post-pull config reload
    cmd_install_skills.go    ralph install-skills - install ralph's bundled Claude Code skills into ~/.claude/skills/
    cmd_migrate.go           ralph migrate - update broken symlinks (--status for plan preview)
    cmd_outdated.go          ralph outdated - check for newer versions of packages
    cmd_version.go           ralph version (-o json emits the four-field build object — cross-tool convention)

configs/examples/
  examples.go                Embeds default.config.toml, the starter config ralph init writes

internal/
  binversion/
    binversion.go            Probe an installed binary via `<bin> version -o json` (cross-tool version convention)
  buildinfo/
    buildinfo.go             Link-time build metadata (version/commit/tag/build_time) + debug.ReadBuildInfo fallback
  buildstate/
    state.go                 Build and package run state in ~/.config/ralph/.builds_state (JSON)
  config/
    types.go                 Config, Dotfile, Repo, Tool, Package, ShellConfig structs (TOML)
    load.go                  LoadConfig: resolve RALPH_CONFIG or the XDG path, overlay config.local.toml, merge recipes
    local.go                 config.local.toml overlay: path and merge onto the main config
    validate.go              ValidateConfig, ValidateMergedConfig, ExpandPath
    deps.go                  Topological sort (Kahn's algorithm) for depends_on ordering
    graph.go                 BuildRecipeGraph: recipes grouped by wave for ralph graph
    timeouts.go              DefaultExecTimeout for build, package and git commands
    enable.go                IsEnabled (*bool pattern: nil/true=enabled)
    host.go                  Host and profile filtering (ShouldApplyForHost, ShouldApplyForProfiles)
    filter.go                SkipReason/Applies: the enable, host and profile check for one item
    recipe.go                Recipe loading, discovery, and merging
    sources.go               Remote recipe sources ([[recipe_sources]]): cache under ~/.config/ralph/sources/
    source_overrides.go      overrides.toml a source ships: its [recipes_config.overrides] layered between config.toml and config.local.toml
    overrides.go             Set/remove recipe overrides in config.toml (text-level, backup+validate)
    vars.go                  Recipe variables: {{vars.<name>}} expansion in shell items ([recipe.vars] defaults + override values)
    migrate.go               MigrateFromLegacy (dotter → ralph)
  dotfile/
    symlink.go               Create/update symlinks and dir symlinks
    copy.go                  Copy files
    mkdir.go                 Create directories
    template.go              Go template processing
  gitutil/
    gitutil.go               Commit hash, subtree tree hash (GetTreeHash), scoped dirty checks, safe URL/ref checks
    sync.go                  Clone, fetch, checkout, pull helpers shared by repos, packages and sources
  lockfile/
    lockfile.go              Run lock: advisory flock on ~/.config/ralph/.lock, fails fast on contention
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
    update.go                SyncPackages (clone/pull) and BuildPackage (change detection, build, install)
    outdated.go              CheckOutdated for go-install, remote, and make packages
    status.go                CheckPackageStatuses: read-only package status for ralph list
  progress/
    progress.go              Progress counters and status lines; silenced under -o json
  skills/
    install.go               Install the embedded bundled/ skills into ~/.claude/skills/
  state/
    recipe_state.go          Recipe artifact manifest (.recipe_state): load/save, Diff, CarryOrphans, MergeRetry
    safe_delete.go           SafeRemove: the guarded delete every cleanup removal goes through
  testutil/
    testutil.go              Test helpers: temp HOME, ralph config dir, git commands
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
- Git operations via `os/exec`, mostly in `internal/gitutil/` (used by `internal/repo/`, packages and recipe sources)
- Dry-run: `--dry-run`/`-n` global flag, threaded through all operations; implies --verbose
- Build state tracked in `~/.config/ralph/.builds_state` (JSON), packages use `pkg:` prefix keys; idempotent builds also record a content hash there
- `run = "once"` builds and remote/make/local packages detect changes via the **subtree tree-hash** of `working_dir` (`git rev-parse HEAD:<subdir>`), not the repo-wide commit — so commits elsewhere in the repo don't force a rebuild; the scoped dirty check (`git status --porcelain -- .`) excludes gitignored build output. See `internal/gitutil` (`GetTreeHash`, `HasGitChangesInPath`).
- Build hooks support either inline commands or a script file (mutually exclusive)
- Exec timeouts: every external command runs through `internal/execx` under a context with a timeout. Builds and packages use their `timeout` field (seconds, default 600, `execx.DefaultTimeout`); lifecycle hooks and git network calls get 600 s, local git queries 2 min, tool checks 30 s. Builds, package builds, hooks and tool checks use `execx.Command`: own session (`Setsid`), no controlling terminal (a `/dev/tty` prompt such as sudo fails at once), and a timeout sends SIGTERM to the whole process group, then SIGKILL after a 2 s grace. Git uses `execx.TerminalCommand`: it stays in ralph's process group so ssh and credential prompts work, a timeout signals git alone, and `WaitDelay` bounds `Wait` when ssh keeps the pipe open. `stopOnSignal` (`cmd/ralph/commands/root.go`) catches SIGINT/SIGTERM/SIGHUP, stops every running command with `execx.TerminateAll` (which also refuses new starts) and exits 128+signal
- Dependency ordering: `depends_on` on builds and packages; topological sort (Kahn's algorithm) determines execution order in a unified phase
- Package sources: `local`, `remote`, `make` (remote + default make build/install), `go-install` (go install module@version)
- Recipe artifact manifest tracked in `~/.config/ralph/.recipe_state` (JSON); written on **every** successful `ralph up`/`apply` (not only under `--enable-cleanup`, so the cleanup baseline never goes stale: `recordManifestAndCleanup`), consumed by the cleanup phase, inspectable via `ralph state show`. A run that removes nothing (cleanup off, or skipped) keeps the orphans `state.Diff` reports in the saved manifest (`RecipeState.CarryOrphans`, decided in `settleOrphans`): a recipe gone entirely is kept whole with its uninstall hooks and `pending_removal` set, a partial orphan's dropped artifacts stay in its entry, so the next cleanup still removes them. A `.recipe_state` that does not parse (`state.ErrCorrupt`) is moved aside to `.recipe_state.corrupt-<UTC time>` (`state.MoveCorruptAside`) and the run goes on from an empty state, so it removes nothing; a read error keeps the file and skips the save
- Cleanup safety invariants (SafeRemove): never deletes any path (symlink, copy, dir, or `install_path`) still declared by the active config, whether a recipe owns it or it sits straight in `config.toml` (guard set from `protectedPaths`: `RecipeState.AllPaths` plus the ownerless paths `buildIntendedManifest` returns, computed before any `--recipe` filtering and before orphans are carried), never deletes the running `ralph` binary or a symlink to it (`ErrSelfDelete`, via `os.Executable()`); package clone dirs and repos are abandon-with-log, never auto-removed
- Cleanup lifecycle invariants: uninstall hooks (`pre/post_uninstall`) run only when a recipe is removed **entirely** — a partial orphan (recipe still present, one artifact dropped) does not fire them; a path whose removal fails is re-tracked (`RecipeState.MergeRetry`, hooks dropped) so the next run retries instead of leaking; `ralph clean --recipe X` saves the on-disk state with only X's entry replaced (`scopedState`), so X's hooks do not run again
- Cleanup freezes rather than removes when the config may be incomplete. Discovery records a recipes directory it finds missing (auto-discovered local `recipes/`, or a recipe source's recipes dir) in `cfg.MissingRecipeDirs`. `keepFrozenRecipes` then carries the previous state's recipes from that origin forward untouched and warns on stderr; `ralph clean` shares it. Local recipes record an empty `recipe_source`, so the empty origin is the dotfiles repo. A failed sync step in the same `ralph up` (dotfiles pull, recipe source pull, package sync) skips removal with a cleanup-phase warning and still saves state. Explicit `[[recipes]]` refs need no guard: a missing on-host recipe file fails config loading
- Self-healing apply: a missing declared `install_path` forces a package rebuild on a normal `ralph up` (`firstMissingInstallPath`), so a deleted binary recovers without `--reset-builds`
- Packages: `[packages]` config section — `ralph up` pulls and builds in one step
- Recipe source profiles gate the entire remote source before checkout, discovery, sync, and fingerprinting; `.recipe_state` records each remote recipe's source so cleanup freezes by provenance rather than by a name prefix. Version-0 state is migrated once using the exact `<source>/` namespace, conservatively preserving ambiguous legacy entries.
- Recipe sources may ship an `overrides.toml` at the checkout root carrying `[recipes_config.overrides.<key>]` tables (nothing else — unknown keys fail loading). Override precedence per key: `config.toml` < active sources < `config.local.toml`; two sources overriding the same key is an error. Source overrides are collected (`prepareRecipeSources`) before any recipe is discovered, so a source can override local recipes and other sources' recipes alike; `cfg.SourceOverrideOrigins` records which source set what
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
make install_deps   # pinned goimports, golines, gofumpt and golangci-lint (versions in the Makefile)
make lint           # golangci-lint
make format         # goimports + golines @100 (gofumpt base)
make format-check   # what CI runs; fails if a file needs formatting or a formatter is missing
```

CI (`.github/workflows/ci.yml`) installs the same pinned formatters, runs `make format-check`, `go vet`, golangci-lint and `go test -race ./...` on every PR and push, and the Docker integration suites on pushes to main. The release workflow runs the unit tests before GoReleaser. Bump a tool by editing its `*_VERSION` variable in the Makefile; Dependabot covers `go.mod` and the workflow actions, not these.

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

To test against a scratch config, give the config a directory of its own. ralph keeps its state next to the config file (see "Where state lives"), so a scratch config never reads or writes the real state:

```bash
mkdir -p /tmp/ralph-test/dots
cat > /tmp/ralph-test/config.toml << 'EOF'
dotfiles_repo_path = "/tmp/ralph-test/dots"
[dotfiles.test]
source = "test.txt"
target = "/tmp/ralph-test/target.txt"
EOF
touch /tmp/ralph-test/dots/test.txt
RALPH_CONFIG=/tmp/ralph-test/config.toml ./ralph up --dry-run
```

A run without `--dry-run` still edits your shell rc file and expands `~` targets against `$HOME`. For a real scratch run, point `HOME` at a temp dir as well (`--no-sync` because the scratch dotfiles dir is not a git repo to pull):

```bash
HOME=$(mktemp -d) RALPH_CONFIG=/tmp/ralph-test/config.toml ./ralph up --no-sync
```

ralph reads config from `RALPH_CONFIG`, else `$XDG_CONFIG_HOME/ralph/config.toml`, else `~/.config/ralph/config.toml`. `RALPH_CONFIG=<new path> ralph init` creates the file at that path.

### Where state lives

The state directory is `$RALPH_STATE_DIR` when set, else the directory that holds the config file: `~/.config/ralph` by default (`$XDG_CONFIG_HOME/ralph` when that is set), or the directory of `RALPH_CONFIG`. `paths.StateDir` (`internal/paths`) resolves it and every path below the config row goes through it. `ralph config` prints it.

| Path | Contents |
|---|---|
| `~/.config/ralph/config.toml` | User config (or `RALPH_CONFIG`) |
| `<state dir>/.builds_state` | Build + package change-detection hashes (JSON) |
| `<state dir>/.recipe_state` | Recipe artifact manifest: what each recipe owns (JSON); drives cleanup |
| `<state dir>/.lock` | Run lock held by `up`, `apply`, `sync` and `clean`; a dry run takes it only if the file exists and never creates it (`lockfile.AcquireExisting`) |
| `<state dir>/generated/` | Generated shell files: `generated_aliases.sh`, `generated_functions.sh`, `generated_env.sh` |
| `<state dir>/pkg/` | Default clone dir for remote/make packages (an explicit `packages_dir` wins) |
| `<state dir>/sources/` | Cached checkouts of remote recipe sources (`[[recipe_sources]]`) |

State used to live in `~/.config/ralph` whatever the config path. When the state directory has no `.recipe_state` but `~/.config/ralph` has one, ralph prints one warning on stderr that names both directories; the fix is `RALPH_STATE_DIR=~/.config/ralph` or moving the files. Unit tests drop `RALPH_CONFIG`, `RALPH_STATE_DIR` and `XDG_CONFIG_HOME` in `TestMain` (`testutil.ClearRalphEnv`), so a test that points `HOME` at a temp dir keeps all state inside it.

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

`ralph up` and `ralph apply` both run `runApplyCommand` (`sequence.go`): it takes the run lock, loads the config, and for `up` runs `runSyncSequence` first (dotfiles pull, recipe sources, package sync), reloading the config when the pull moved the dotfiles repo or a recipe source. It then runs `runApplySequence`, which iterates the merged config in phases (see Apply Execution Order above). `ralph sync` runs `runSyncSequence` alone, so `apply` never syncs and `sync` never applies. Each phase is a loop over the relevant config items, each item recording a `report.StepResult`. The report collects all results and prints a summary at the end. Items skip on `config.SkipReason` (enable, host, profile, in that order).

The cleanup phase (`--enable-cleanup`) is a diff between:
- the current run's intended manifest (all artifacts the active config would produce), and
- the previous run's recorded manifest from `.recipe_state`.

Items in the recorded manifest that aren't in the intended manifest are orphans. Frozen recipes (host or profile filtered, or recorded from a recipes directory that is now missing) are copied into the intended manifest first, so they never become orphans. `SafeRemove` deletes them with safety rails: checks the path is still not claimed by any active recipe (`RecipeState.AllPaths`), refuses to delete the running ralph binary (`ErrSelfDelete`), and refuses to delete outside HOME-prefixed paths. Package clone dirs and repos are abandoned (logged, not removed).

### Adding a new recipe-declaration type

1. Add the struct to `internal/config/types.go` (e.g. `MyThing struct { ... }`).
2. Add a map field to `Config` and/or `Recipe` (e.g. `MyThings map[string]MyThing`).
3. Implement the apply logic as a new phase function. Follow the pattern of existing phases: iterate the map, return `[]report.StepResult`, handle dry-run.
4. Wire it into `runApplySequence` in `sequence.go`; `ralph up` and `ralph apply` share it.
5. Add a `ralph list` subcommand or extend the existing one if the type should be listed.
6. Write a unit test and, if state-bearing, an integration test.

### Debugging a failed apply

1. Run with `--verbose` or `--dry-run` to see per-item output.
2. Check `ralph doctor`: it catches broken symlinks, leftover dangling links in directories recipes link into (`doctor_dangling.go`), missing tools, uncloned remotes, and bad build state.
   It also flags unparseable or too-new state files (`doctor_state.go`), missing git (`doctor_git.go`), and recipe source checkouts that are not git checkouts or recipes dirs that are missing (`doctor_sources.go`).
3. Inspect build state: `ralph state show` lists what each recipe owns and when it last built.
4. Force a rebuild: `ralph up --force` re-runs every package and `once` build regardless of hash.
5. Clear all build state: `ralph up --reset-builds` (not needed to recover a deleted binary — a missing `install_path` self-heals on a plain `ralph up`).
