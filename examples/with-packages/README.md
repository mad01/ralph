# Packages Ralph Example

A ralph setup that uses the packages feature to manage tools built from source. Packages can be either remote (cloned from a git repository) or local (built from a directory already on disk).

## What is included

- **Remote package (fzf)**: Cloned from GitHub, built, and installed to `~/.local/bin/`.
- **Local package (local-tool)**: Built from a local project directory.
- **Recipe-defined package (delta)**: A remote package defined inside a recipe, showing that packages work in recipes too.
- **Build script**: A helper script demonstrating how to wrap build logic.

## Directory structure

```
with-packages/
  config.toml                      Main config with packages
  recipes/
    tools/
      recipe.toml                  Recipe that defines additional packages
  scripts/
    build-local-tool.sh            Helper build script
```

## How packages work

Packages are managed with `ralph sync` (to pull remote repos) and `ralph apply` (to build). Remote packages are cloned into `packages_dir` (default `~/.config/ralph/pkg/`), then built and installed using the specified commands. Local packages skip the clone step and run build/install from their `working_dir`.

- `ralph sync` -- pull dotfiles repo and clone/pull remote packages.
- `ralph apply` -- build packages that have changed (alongside all other apply operations).
- `ralph apply --force` -- force rebuild all packages.
- `ralph list --source remote` -- list only remote packages.

## Getting started

1. Copy `config.toml` to `~/.config/ralph/config.toml`.
2. Ensure `~/.local/bin` is on your `PATH`.
3. Run `ralph sync && ralph apply` to clone, build, and install packages.

For details on package management, see the [Packages guide](../../docs/packages.md).
