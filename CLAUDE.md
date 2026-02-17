# CLAUDE.md - Ralph

A Go CLI tool for managing dotfiles and shell configurations. Uses a TOML config file to define symlinks, copies, aliases, functions, env vars, repos, and build hooks. Named after Ralph Wiggum.

## Quick Reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Install | `make install` |
| Test | `make test` |
| Integration tests | `make test-integration` |
| Lint | `make lint` |
| Format | `make format` |
| Run | `./ralph apply` |
| Sandbox | `make sandbox` |

## Architecture

```
cmd/ralph/
  main.go                    Thin entry point, calls commands.Execute()
  commands/
    root.go                  Cobra root command + global flags (--dry-run, --verbose, --quiet)
    cmd_apply.go             ralph apply - main operation
    cmd_init.go              ralph init - interactive config creation
    cmd_add.go               ralph add - add dotfiles
    cmd_list.go              ralph list - show managed items
    cmd_doctor.go            ralph doctor - health checks
    cmd_migrate.go           ralph migrate - update broken symlinks
    cmd_version.go           ralph version

internal/
  config/
    types.go                 Config, Dotfile, Repo, Tool, ShellConfig structs (TOML)
    load.go                  LoadConfig from XDG path
    validate.go              ValidateConfig, ValidateMergedConfig, ExpandPath
    enable.go                IsEnabled (*bool pattern: nil/true=enabled)
    host.go                  Host filtering (ShouldApplyForHost)
    recipe.go                Recipe loading, discovery, and merging
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
    builds.go                Build hooks with run modes (always/once/manual), git hash tracking
  repo/
    clone.go                 Git clone/pull/checkout via os/exec
  migrate/
    migrate.go               Symlink migration after repo reorganization
  report/
    report.go                Structured run reporting with phases and step results
  tool/
    status.go                Tool check status via sh -c

pkg/pipeutil/                Public utility for pipe-based I/O
```

## Conventions

- Config: TOML via `github.com/BurntSushi/toml`, lives at `~/.config/ralph/config.toml`
- CLI: `github.com/spf13/cobra`, each command in its own `cmd_*.go`, registered via `init()`
- Enable pattern: `*bool` field — nil/true = enabled, false = disabled
- Host filtering: `hosts` field on most items — empty = all hosts
- Recipes: modular `recipe.toml` files, auto-discovered or explicit references
- Git operations via `os/exec` in `internal/repo/`
- Dry-run: `--dry-run`/`-n` global flag, threaded through all operations
- Build state tracked in `~/.config/ralph/.builds_state` (JSON)
- Generated shell scripts in `~/.config/ralph/generated/`
- Version embedded via `-ldflags` from git commit hash
- Integration tests run in Docker containers (`tests/integration/`)

## Apply Execution Order

1. Legacy migration (dotter → ralph config)
2. Pre-apply hooks
3. Directories
4. Repositories (clone/update)
5. Dotfiles (symlink/copy/template)
6. Shell configuration (generate alias+function files, inject source lines)
7. Tool checks
8. Build hooks
9. Post-apply hooks
10. Print report summary

## Key Files

- `config.toml` — user's dotfiles configuration (at `~/.config/ralph/config.toml`)
- `examples/dotfiles-repo/` — example dotfiles repository structure
- `configs/examples/` — default config templates
- `tests/integration/` — Docker-based integration test scripts
