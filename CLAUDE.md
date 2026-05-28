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
    cmd_down.go              ralph down - uninstall a recipe
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
    cmd_version.go           ralph version

internal/
  config/
    types.go                 Config, Dotfile, Repo, Tool, Package, ShellConfig structs (TOML)
    load.go                  LoadConfig from XDG path
    validate.go              ValidateConfig, ValidateMergedConfig, ExpandPath
    deps.go                  Topological sort (Kahn's algorithm) for depends_on ordering
    enable.go                IsEnabled (*bool pattern: nil/true=enabled)
    host.go                  Host filtering (ShouldApplyForHost)
    recipe.go                Recipe loading, discovery, and merging
    overrides.go             Set/remove recipe overrides in config.toml (text-level, backup+validate)
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
    hooks.go                 Run lifecycle hooks (pre/post apply/link/uninstall)
    builds.go                Build hooks with run modes (always/once/manual), git hash tracking
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
- Recipes: modular `recipe.toml` files, auto-discovered or explicit references
- Git operations via `os/exec` in `internal/repo/`
- Dry-run: `--dry-run`/`-n` global flag, threaded through all operations; implies --verbose
- Build state tracked in `~/.config/ralph/.builds_state` (JSON), packages use `pkg:` prefix keys; idempotent builds also record a content hash there
- Build hooks support either inline commands or a script file (mutually exclusive)
- Exec timeouts: `timeout` field (seconds, default 600) on builds and packages; all exec.Command calls use context.WithTimeout
- Dependency ordering: `depends_on` on builds and packages; topological sort (Kahn's algorithm) determines execution order in a unified phase
- Package sources: `local`, `remote`, `make` (remote + default make build/install), `go-install` (go install module@version)
- Recipe artifact manifest tracked in `~/.config/ralph/.recipe_state` (JSON); written by `ralph up --enable-cleanup`, consumed by the cleanup phase, inspectable via `ralph state show`
- Packages: `[packages]` config section — `ralph up` pulls and builds in one step
- Package clone dir: `packages_dir` config field (default: `~/.config/ralph/pkg/`)
- Generated shell scripts in `~/.config/ralph/generated/` (generated_aliases.sh, generated_functions.sh, generated_env.sh)
- Version embedded via `-ldflags` from git commit hash
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
  workflows.md           Daily usage patterns (up/down/doctor)
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
